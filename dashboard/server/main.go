package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var (
	// Prometheus metrics
	activeConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dashboard_active_connections",
		Help: "Number of active WebSocket connections",
	})
	messagesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dashboard_messages_sent_total",
		Help: "Total number of messages sent to clients",
	})
	errorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dashboard_errors_total",
		Help: "Total number of errors encountered",
	})
	latencyHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "dashboard_message_latency_ms",
		Help:    "Message processing latency in milliseconds",
		Buckets: prometheus.DefBuckets,
	})
)

func init() {
	prometheus.MustRegister(activeConnections)
	prometheus.MustRegister(messagesSent)
	prometheus.MustRegister(errorsTotal)
	prometheus.MustRegister(latencyHistogram)
}

// Hub maintains active client connections
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// Client represents a WebSocket connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	subscriptions map[string]bool
	mu   sync.RWMutex
}

// Message represents a WebSocket message
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Server represents the dashboard server
type Server struct {
	hub        *Hub
	nc         *nats.Conn
	logger     *zap.Logger
	collectors map[string]*DataCollector
}

// DataCollector collects and aggregates data
type DataCollector struct {
	name       string
	data       []interface{}
	maxSize    int
	mu         sync.RWMutex
	lastUpdate time.Time
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Configure CORS properly in production
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			activeConnections.Inc()
			log.Printf("Client connected. Total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			activeConnections.Dec()
			log.Printf("Client disconnected. Total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
					messagesSent.Inc()
				default:
					// Client's send channel is full, close it
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		errorsTotal.Inc()
		return
	}

	client := &Client{
		hub:  s.hub,
		conn: conn,
		send: make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle different message types
		switch msg.Type {
		case "subscribe":
			var streams []string
			if err := json.Unmarshal(msg.Data, &streams); err == nil {
				c.mu.Lock()
				for _, stream := range streams {
					c.subscriptions[stream] = true
				}
				c.mu.Unlock()
				log.Printf("Client subscribed to: %v", streams)
			}
		case "unsubscribe":
			var streams []string
			if err := json.Unmarshal(msg.Data, &streams); err == nil {
				c.mu.Lock()
				for _, stream := range streams {
					delete(c.subscriptions, stream)
				}
				c.mu.Unlock()
			}
		case "ping":
			// Send pong
			response := Message{Type: "pong"}
			if data, err := json.Marshal(response); err == nil {
				c.send <- data
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) setupNATSSubscriptions() error {
	// Subscribe to order updates
	_, err := s.nc.Subscribe("orders.*", func(msg *nats.Msg) {
		s.handleOrderUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to orders: %w", err)
	}

	// Subscribe to position updates
	_, err = s.nc.Subscribe("positions.*", func(msg *nats.Msg) {
		s.handlePositionUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to positions: %w", err)
	}

	// Subscribe to market data
	_, err = s.nc.Subscribe("market.*", func(msg *nats.Msg) {
		s.handleMarketUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to market: %w", err)
	}

	// Subscribe to system metrics
	_, err = s.nc.Subscribe("system.metrics", func(msg *nats.Msg) {
		s.handleSystemMetrics(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to system metrics: %w", err)
	}

	// Subscribe to risk metrics
	_, err = s.nc.Subscribe("risk.*", func(msg *nats.Msg) {
		s.handleRiskUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to risk: %w", err)
	}

	return nil
}

func (s *Server) handleOrderUpdate(data []byte) {
	start := time.Now()
	defer func() {
		latencyHistogram.Observe(float64(time.Since(start).Milliseconds()))
	}()

	message := Message{
		Type: "order_update",
		Data: data,
	}

	if msgData, err := json.Marshal(message); err == nil {
		s.broadcastToSubscribers("orders", msgData)
		s.collectors["orders"].addData(data)
	}
}

func (s *Server) handlePositionUpdate(data []byte) {
	message := Message{
		Type: "position_update",
		Data: data,
	}

	if msgData, err := json.Marshal(message); err == nil {
		s.broadcastToSubscribers("positions", msgData)
		s.collectors["positions"].addData(data)
	}
}

func (s *Server) handleMarketUpdate(data []byte) {
	message := Message{
		Type: "market_update",
		Data: data,
	}

	if msgData, err := json.Marshal(message); err == nil {
		s.broadcastToSubscribers("market", msgData)
		s.collectors["market"].addData(data)
	}
}

func (s *Server) handleSystemMetrics(data []byte) {
	message := Message{
		Type: "system_metrics",
		Data: data,
	}

	if msgData, err := json.Marshal(message); err == nil {
		s.broadcastToSubscribers("system", msgData)
		s.collectors["system"].addData(data)
	}
}

func (s *Server) handleRiskUpdate(data []byte) {
	message := Message{
		Type: "risk_update",
		Data: data,
	}

	if msgData, err := json.Marshal(message); err == nil {
		s.broadcastToSubscribers("risk", msgData)
		s.collectors["risk"].addData(data)
	}
}

func (s *Server) broadcastToSubscribers(stream string, message []byte) {
	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()

	for client := range s.hub.clients {
		client.mu.RLock()
		subscribed := client.subscriptions[stream]
		client.mu.RUnlock()

		if subscribed {
			select {
			case client.send <- message:
			default:
				// Client channel is full, skip
			}
		}
	}
}

func (dc *DataCollector) addData(data []byte) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.data = append(dc.data, data)
	if len(dc.data) > dc.maxSize {
		dc.data = dc.data[1:]
	}
	dc.lastUpdate = time.Now()
}

func (s *Server) startMetricsGenerator() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Generate mock system metrics for demo
		metrics := map[string]interface{}{
			"cpu":               45.2 + (time.Now().Unix() % 20),
			"memory":            2048 + (time.Now().Unix() % 512),
			"latency":           0.5 + float64(time.Now().Unix()%10)/10,
			"ordersPerSecond":   150 + (time.Now().Unix() % 50),
			"activeConnections": len(s.hub.clients),
		}

		if data, err := json.Marshal(metrics); err == nil {
			s.handleSystemMetrics(data)
		}
	}
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}
	defer nc.Close()

	hub := newHub()
	go hub.run()

	server := &Server{
		hub:    hub,
		nc:     nc,
		logger: logger,
		collectors: map[string]*DataCollector{
			"orders":    {name: "orders", maxSize: 1000},
			"positions": {name: "positions", maxSize: 100},
			"market":    {name: "market", maxSize: 500},
			"system":    {name: "system", maxSize: 100},
			"risk":      {name: "risk", maxSize: 100},
		},
	}

	// Setup NATS subscriptions
	if err := server.setupNATSSubscriptions(); err != nil {
		log.Fatal("Failed to setup NATS subscriptions:", err)
	}

	// Start metrics generator for demo
	go server.startMetricsGenerator()

	// Setup HTTP routes
	http.HandleFunc("/ws", server.handleWebSocket)
	http.Handle("/metrics", promhttp.Handler())
	
	// Serve static files
	http.Handle("/", http.FileServer(http.Dir("./dashboard/frontend/build")))

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// API endpoints
	http.HandleFunc("/api/history", server.handleHistoryRequest)
	http.HandleFunc("/api/summary", server.handleSummaryRequest)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      nil,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.Shutdown(ctx)
	}()

	logger.Info("Dashboard server starting", zap.String("addr", srv.Addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Server failed:", err)
	}
}

func (s *Server) handleHistoryRequest(w http.ResponseWriter, r *http.Request) {
	stream := r.URL.Query().Get("stream")
	if stream == "" {
		http.Error(w, "stream parameter required", http.StatusBadRequest)
		return
	}

	collector, exists := s.collectors[stream]
	if !exists {
		http.Error(w, "unknown stream", http.StatusNotFound)
		return
	}

	collector.mu.RLock()
	data := collector.data
	collector.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stream": stream,
		"data":   data,
		"count":  len(data),
	})
}

func (s *Server) handleSummaryRequest(w http.ResponseWriter, r *http.Request) {
	summary := map[string]interface{}{
		"activeConnections": len(s.hub.clients),
		"uptime":           time.Since(time.Now()).Seconds(),
		"streams": map[string]interface{}{
			"orders":    len(s.collectors["orders"].data),
			"positions": len(s.collectors["positions"].data),
			"market":    len(s.collectors["market"].data),
			"system":    len(s.collectors["system"].data),
			"risk":      len(s.collectors["risk"].data),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}