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
		origin := r.Header.Get("Origin")
		// Allow specific origins
		allowedOrigins := []string{
			"http://localhost:3000",
			"http://localhost:8080",
			// Add your production domains here
		}
		
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
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

func (s *Server) setupRealOmsSubscriptions() error {
	s.logger.Info("Setting up real OMS NATS subscriptions")

	// Subscribe to real OMS order events
	_, err := s.nc.Subscribe("order.event.*", func(msg *nats.Msg) {
		s.handleOrderUpdate(msg.Data)
		s.logger.Debug("Received order event", zap.String("subject", msg.Subject))
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to order events: %w", err)
	}

	// Subscribe to position updates
	_, err = s.nc.Subscribe("position.update.*", func(msg *nats.Msg) {
		s.handlePositionUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to position updates: %w", err)
	}

	// Subscribe to market data from OMS
	_, err = s.nc.Subscribe("market.data.*", func(msg *nats.Msg) {
		s.handleMarketUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to market data: %w", err)
	}

	// Subscribe to risk updates
	_, err = s.nc.Subscribe("risk.metrics.*", func(msg *nats.Msg) {
		s.handleRiskUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to risk metrics: %w", err)
	}

	// Subscribe to system health
	_, err = s.nc.Subscribe("oms.health.*", func(msg *nats.Msg) {
		s.handleSystemHealth(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to system health: %w", err)
	}

	// Subscribe to trade executions
	_, err = s.nc.Subscribe("trade.executed.*", func(msg *nats.Msg) {
		s.handleTradeExecution(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to trade executions: %w", err)
	}

	s.logger.Info("Successfully subscribed to all OMS events")
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

func (s *Server) handleSystemHealth(data []byte) {
	// Transform OMS health data to dashboard format
	var omsHealth struct {
		Services []struct {
			Name   string  `json:"name"`
			Status string  `json:"status"`
			Latency float64 `json:"latency_ms"`
		} `json:"services"`
		Metrics struct {
			CPU         float64 `json:"cpu_percent"`
			Memory      int64   `json:"memory_mb"`
			Connections int     `json:"connections"`
		} `json:"metrics"`
	}

	if err := json.Unmarshal(data, &omsHealth); err == nil {
		// Transform to dashboard format
		dashboardData := map[string]interface{}{
			"cpu":               omsHealth.Metrics.CPU,
			"memory":            2048, // Total memory
			"usedMemory":        omsHealth.Metrics.Memory,
			"activeConnections": omsHealth.Metrics.Connections,
			"services":          omsHealth.Services,
		}

		transformedData, _ := json.Marshal(dashboardData)
		message := Message{
			Type: "system_metrics",
			Data: transformedData,
		}

		if msgData, err := json.Marshal(message); err == nil {
			s.broadcastToSubscribers("system", msgData)
			s.collectors["system"].addData(transformedData)
		}
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

func (s *Server) handleTradeExecution(data []byte) {
	// Forward trade execution as both order update and position update
	message := Message{
		Type: "trade_executed",
		Data: data,
	}

	if msgData, err := json.Marshal(message); err == nil {
		s.broadcastToSubscribers("orders", msgData)
		s.broadcastToSubscribers("positions", msgData)
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

// Periodically request system metrics from OMS
func (s *Server) startMetricsPoller() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Request system metrics from OMS
		s.nc.Request("oms.metrics.request", []byte(`{"type":"system"}`), 1*time.Second)
		
		// Request aggregated position data
		s.nc.Request("oms.positions.summary", []byte(`{}`), 1*time.Second)
	}
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Get NATS URL from environment or use default
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
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

	// Setup real OMS subscriptions
	if err := server.setupRealOmsSubscriptions(); err != nil {
		log.Fatal("Failed to setup OMS subscriptions:", err)
	}

	// Start metrics poller
	go server.startMetricsPoller()

	// Setup HTTP routes
	http.HandleFunc("/ws", server.handleWebSocket)
	http.Handle("/metrics", promhttp.Handler())
	
	// Serve static files
	http.Handle("/", http.FileServer(http.Dir("../frontend/build")))

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Check NATS connection
		if !nc.IsConnected() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy","reason":"NATS disconnected"}`))
			return
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// API endpoints
	http.HandleFunc("/api/history", server.handleHistoryRequest)
	http.HandleFunc("/api/summary", server.handleSummaryRequest)
	http.HandleFunc("/api/config", server.handleConfigRequest)

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

		logger.Info("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.Shutdown(ctx)
	}()

	logger.Info("Dashboard server starting", 
		zap.String("addr", srv.Addr),
		zap.String("nats", natsURL))
	
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
		"natsConnected":     s.nc.IsConnected(),
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

func (s *Server) handleConfigRequest(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"version":     "1.0.0",
		"environment": os.Getenv("ENVIRONMENT"),
		"features": map[string]bool{
			"realtime_orders":    true,
			"position_tracking":  true,
			"risk_management":    true,
			"system_monitoring":  true,
		},
		"nats_subjects": map[string]string{
			"orders":    "order.event.*",
			"positions": "position.update.*",
			"market":    "market.data.*",
			"risk":      "risk.metrics.*",
			"health":    "oms.health.*",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}