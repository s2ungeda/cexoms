package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Dashboard provides a real-time monitoring dashboard
type Dashboard struct {
	mu sync.RWMutex
	
	// Data sources
	logger             *Logger
	metricsCollector   *MetricsCollector
	pnlTracker         *StrategyPnLTracker
	arbitrageAnalyzer  *ArbitrageAnalyzer
	
	// WebSocket connections
	connections        map[string]*DashboardConnection
	broadcast          chan *DashboardUpdate
	
	// Configuration
	config             *DashboardConfig
	logger             *zap.Logger
	
	// HTTP server
	server             *http.Server
	router             *mux.Router
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DashboardConfig holds dashboard configuration
type DashboardConfig struct {
	Port               int
	UpdateInterval     time.Duration
	WebSocketTimeout   time.Duration
	MaxConnections     int
	EnableAuth         bool
	AuthToken          string
	StaticFilesPath    string
}

// DefaultDashboardConfig returns default configuration
func DefaultDashboardConfig() *DashboardConfig {
	return &DashboardConfig{
		Port:             8080,
		UpdateInterval:   1 * time.Second,
		WebSocketTimeout: 60 * time.Second,
		MaxConnections:   100,
		EnableAuth:       false,
		AuthToken:        "",
		StaticFilesPath:  "./web/static",
	}
}

// DashboardConnection represents a WebSocket connection
type DashboardConnection struct {
	ID         string
	conn       *websocket.Conn
	send       chan []byte
	dashboard  *Dashboard
	filters    DashboardFilters
	lastPing   time.Time
}

// DashboardFilters contains client-side filters
type DashboardFilters struct {
	Accounts   []string
	Strategies []string
	Symbols    []string
	TimeRange  string // "1h", "24h", "7d", "30d"
}

// DashboardUpdate represents an update message
type DashboardUpdate struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// In production, implement proper origin checking
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// NewDashboard creates a new dashboard
func NewDashboard(
	config *DashboardConfig,
	logger *Logger,
	metricsCollector *MetricsCollector,
	pnlTracker *StrategyPnLTracker,
	arbitrageAnalyzer *ArbitrageAnalyzer,
	zapLogger *zap.Logger,
) *Dashboard {
	if config == nil {
		config = DefaultDashboardConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	dashboard := &Dashboard{
		logger:            logger,
		metricsCollector:  metricsCollector,
		pnlTracker:        pnlTracker,
		arbitrageAnalyzer: arbitrageAnalyzer,
		connections:       make(map[string]*DashboardConnection),
		broadcast:         make(chan *DashboardUpdate, 100),
		config:            config,
		logger:            zapLogger,
		ctx:               ctx,
		cancel:            cancel,
	}
	
	// Setup routes
	dashboard.setupRoutes()
	
	return dashboard
}

// Start starts the dashboard server
func (d *Dashboard) Start() error {
	// Start broadcast handler
	d.wg.Add(1)
	go d.handleBroadcasts()
	
	// Start update generator
	d.wg.Add(1)
	go d.generateUpdates()
	
	// Start HTTP server
	d.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", d.config.Port),
		Handler:      d.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	d.logger.Info("Starting dashboard server",
		zap.Int("port", d.config.Port))
	
	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			d.logger.Error("Dashboard server error", zap.Error(err))
		}
	}()
	
	return nil
}

// setupRoutes configures HTTP routes
func (d *Dashboard) setupRoutes() {
	d.router = mux.NewRouter()
	
	// API routes
	api := d.router.PathPrefix("/api/v1").Subrouter()
	
	// Apply auth middleware if enabled
	if d.config.EnableAuth {
		api.Use(d.authMiddleware)
	}
	
	// Dashboard data endpoints
	api.HandleFunc("/overview", d.handleOverview).Methods("GET")
	api.HandleFunc("/accounts", d.handleAccounts).Methods("GET")
	api.HandleFunc("/accounts/{accountId}/metrics", d.handleAccountMetrics).Methods("GET")
	api.HandleFunc("/strategies", d.handleStrategies).Methods("GET")
	api.HandleFunc("/strategies/{strategyId}/report", d.handleStrategyReport).Methods("GET")
	api.HandleFunc("/arbitrage/report", d.handleArbitrageReport).Methods("GET")
	api.HandleFunc("/system/metrics", d.handleSystemMetrics).Methods("GET")
	
	// WebSocket endpoint
	d.router.HandleFunc("/ws", d.handleWebSocket)
	
	// Static files
	d.router.PathPrefix("/").Handler(http.FileServer(http.Dir(d.config.StaticFilesPath)))
}

// authMiddleware checks authentication
func (d *Dashboard) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != "Bearer "+d.config.AuthToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleOverview returns dashboard overview data
func (d *Dashboard) handleOverview(w http.ResponseWriter, r *http.Request) {
	globalMetrics := d.metricsCollector.GetGlobalMetrics()
	
	overview := map[string]interface{}{
		"timestamp": time.Now(),
		"accounts": map[string]interface{}{
			"active": globalMetrics.TotalActiveAccounts.Load(),
			"total":  len(d.metricsCollector.accountMetrics),
		},
		"orders": map[string]interface{}{
			"rate_per_second": globalMetrics.TotalOrdersPerSec.Load(),
			"total_volume":    float64(globalMetrics.TotalTradeVolume.Load()) / 100.0,
		},
		"system": map[string]interface{}{
			"cpu_usage_percent": float64(globalMetrics.CPUUsage.Load()) / 100.0,
			"memory_mb":         globalMetrics.MemoryUsage.Load() / 1024 / 1024,
			"goroutines":        globalMetrics.GoroutineCount.Load(),
			"messages_processed": globalMetrics.MessagesProcessed.Load(),
		},
	}
	
	// Add strategy overview
	strategies := d.pnlTracker.GetAllStrategiesSnapshot()
	totalPnL := 0.0
	activeStrategies := 0
	
	for _, strategy := range strategies {
		if sm, ok := strategy.(map[string]interface{}); ok {
			if pnl, ok := sm["total_pnl"].(float64); ok {
				totalPnL += pnl
			}
			if positions, ok := sm["active_positions"].(int); ok && positions > 0 {
				activeStrategies++
			}
		}
	}
	
	overview["strategies"] = map[string]interface{}{
		"total":  len(strategies),
		"active": activeStrategies,
		"total_pnl": totalPnL,
	}
	
	// Add arbitrage overview
	arbMetrics := d.arbitrageAnalyzer.GetMetrics()
	overview["arbitrage"] = map[string]interface{}{
		"opportunities": arbMetrics.TotalOpportunities,
		"executions":    arbMetrics.TotalExecutions,
		"success_rate":  arbMetrics.SuccessRate,
		"total_profit":  arbMetrics.TotalProfit,
	}
	
	d.sendJSON(w, overview)
}

// handleAccounts returns account list and summary
func (d *Dashboard) handleAccounts(w http.ResponseWriter, r *http.Request) {
	d.metricsCollector.mu.RLock()
	accounts := make([]map[string]interface{}, 0)
	
	for accountID := range d.metricsCollector.accountMetrics {
		summary := d.metricsCollector.GetAccountSummary(accountID)
		accounts = append(accounts, summary)
	}
	d.metricsCollector.mu.RUnlock()
	
	d.sendJSON(w, map[string]interface{}{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// handleAccountMetrics returns detailed metrics for an account
func (d *Dashboard) handleAccountMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	accountID := vars["accountId"]
	
	metrics := d.metricsCollector.GetAccountMetrics(accountID)
	if metrics == nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	
	summary := d.metricsCollector.GetAccountSummary(accountID)
	
	// Add time series data
	summary["order_latency_history"] = metrics.OrderLatencyHistory
	summary["trade_volume_history"] = metrics.TradeVolumeHistory
	summary["exposure_history"] = metrics.ExposureHistory
	
	d.sendJSON(w, summary)
}

// handleStrategies returns strategy list and summary
func (d *Dashboard) handleStrategies(w http.ResponseWriter, r *http.Request) {
	strategies := d.pnlTracker.GetAllStrategiesSnapshot()
	
	d.sendJSON(w, map[string]interface{}{
		"strategies": strategies,
		"count":      len(strategies),
	})
}

// handleStrategyReport returns detailed strategy report
func (d *Dashboard) handleStrategyReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	strategyID := vars["strategyId"]
	
	report, err := d.pnlTracker.GetStrategyReport(strategyID)
	if err != nil || report == nil {
		http.Error(w, "Strategy not found", http.StatusNotFound)
		return
	}
	
	d.sendJSON(w, report)
}

// handleArbitrageReport returns arbitrage performance report
func (d *Dashboard) handleArbitrageReport(w http.ResponseWriter, r *http.Request) {
	report := d.arbitrageAnalyzer.GetReport()
	d.sendJSON(w, report)
}

// handleSystemMetrics returns system performance metrics
func (d *Dashboard) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	globalMetrics := d.metricsCollector.GetGlobalMetrics()
	
	// Get exchange latencies
	globalMetrics.mu.RLock()
	exchangeLatencies := make(map[string]map[string]interface{})
	for exchange, stats := range globalMetrics.ExchangeLatency {
		exchangeLatencies[exchange] = map[string]interface{}{
			"ping_ms":  float64(stats.PingLatency.Load()) / 1000.0,
			"order_ms": float64(stats.OrderLatency.Load()) / 1000.0,
			"data_ms":  float64(stats.DataLatency.Load()) / 1000.0,
		}
	}
	globalMetrics.mu.RUnlock()
	
	systemMetrics := map[string]interface{}{
		"timestamp": time.Now(),
		"performance": map[string]interface{}{
			"cpu_usage_percent":  float64(globalMetrics.CPUUsage.Load()) / 100.0,
			"memory_bytes":       globalMetrics.MemoryUsage.Load(),
			"goroutines":         globalMetrics.GoroutineCount.Load(),
			"message_queue_depth": globalMetrics.MessageQueueDepth.Load(),
		},
		"throughput": map[string]interface{}{
			"messages_per_second": globalMetrics.MessagesProcessed.Load(),
			"orders_per_second":   globalMetrics.TotalOrdersPerSec.Load(),
			"total_volume_24h":    float64(globalMetrics.TotalTradeVolume.Load()) / 100.0,
		},
		"exchange_latencies": exchangeLatencies,
	}
	
	d.sendJSON(w, systemMetrics)
}

// handleWebSocket handles WebSocket connections
func (d *Dashboard) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check connection limit
	d.mu.RLock()
	if len(d.connections) >= d.config.MaxConnections {
		d.mu.RUnlock()
		http.Error(w, "Connection limit reached", http.StatusServiceUnavailable)
		return
	}
	d.mu.RUnlock()
	
	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		d.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	
	// Create connection
	connID := fmt.Sprintf("conn-%d", time.Now().UnixNano())
	connection := &DashboardConnection{
		ID:        connID,
		conn:      conn,
		send:      make(chan []byte, 256),
		dashboard: d,
		lastPing:  time.Now(),
	}
	
	// Register connection
	d.mu.Lock()
	d.connections[connID] = connection
	d.mu.Unlock()
	
	// Start connection handlers
	go connection.readPump()
	go connection.writePump()
	
	d.logger.Info("WebSocket connection established",
		zap.String("conn_id", connID),
		zap.String("remote_addr", r.RemoteAddr))
}

// readPump handles incoming WebSocket messages
func (c *DashboardConnection) readPump() {
	defer func() {
		c.dashboard.mu.Lock()
		delete(c.dashboard.connections, c.ID)
		c.dashboard.mu.Unlock()
		c.conn.Close()
		close(c.send)
	}()
	
	c.conn.SetReadDeadline(time.Now().Add(c.dashboard.config.WebSocketTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.dashboard.config.WebSocketTimeout))
		c.lastPing = time.Now()
		return nil
	})
	
	for {
		var message map[string]interface{}
		err := c.conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.dashboard.logger.Error("WebSocket read error",
					zap.String("conn_id", c.ID),
					zap.Error(err))
			}
			break
		}
		
		// Handle message
		c.handleMessage(message)
	}
}

// writePump handles outgoing WebSocket messages
func (c *DashboardConnection) writePump() {
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

// handleMessage processes incoming WebSocket messages
func (c *DashboardConnection) handleMessage(message map[string]interface{}) {
	msgType, ok := message["type"].(string)
	if !ok {
		return
	}
	
	switch msgType {
	case "subscribe":
		// Update filters
		if filters, ok := message["filters"].(map[string]interface{}); ok {
			if accounts, ok := filters["accounts"].([]interface{}); ok {
				c.filters.Accounts = make([]string, len(accounts))
				for i, a := range accounts {
					c.filters.Accounts[i] = a.(string)
				}
			}
			if strategies, ok := filters["strategies"].([]interface{}); ok {
				c.filters.Strategies = make([]string, len(strategies))
				for i, s := range strategies {
					c.filters.Strategies[i] = s.(string)
				}
			}
			if symbols, ok := filters["symbols"].([]interface{}); ok {
				c.filters.Symbols = make([]string, len(symbols))
				for i, s := range symbols {
					c.filters.Symbols[i] = s.(string)
				}
			}
			if timeRange, ok := filters["time_range"].(string); ok {
				c.filters.TimeRange = timeRange
			}
		}
		
	case "ping":
		// Send pong
		response := map[string]interface{}{
			"type": "pong",
			"timestamp": time.Now(),
		}
		if data, err := json.Marshal(response); err == nil {
			select {
			case c.send <- data:
			default:
			}
		}
	}
}

// generateUpdates generates periodic dashboard updates
func (d *Dashboard) generateUpdates() {
	defer d.wg.Done()
	
	ticker := time.NewTicker(d.config.UpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.generateUpdate()
		}
	}
}

// generateUpdate creates a dashboard update
func (d *Dashboard) generateUpdate() {
	// Get current metrics
	globalMetrics := d.metricsCollector.GetGlobalMetrics()
	
	// Create update
	update := &DashboardUpdate{
		Type:      "metrics",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"orders_per_second": globalMetrics.TotalOrdersPerSec.Load(),
			"active_accounts":   globalMetrics.TotalActiveAccounts.Load(),
			"cpu_usage":         float64(globalMetrics.CPUUsage.Load()) / 100.0,
			"memory_mb":         globalMetrics.MemoryUsage.Load() / 1024 / 1024,
		},
	}
	
	// Add account metrics
	accountMetrics := make(map[string]interface{})
	d.metricsCollector.mu.RLock()
	for accountID, metrics := range d.metricsCollector.accountMetrics {
		accountMetrics[accountID] = map[string]interface{}{
			"orders_placed":  metrics.OrdersPlaced.Load(),
			"active_positions": metrics.ActivePositions.Load(),
			"total_exposure": float64(metrics.TotalExposure.Load()) / 100.0,
		}
	}
	d.metricsCollector.mu.RUnlock()
	update.Data["accounts"] = accountMetrics
	
	// Add strategy metrics
	strategies := d.pnlTracker.GetAllStrategiesSnapshot()
	update.Data["strategies"] = strategies
	
	// Add arbitrage metrics
	arbMetrics := d.arbitrageAnalyzer.GetMetrics()
	update.Data["arbitrage"] = map[string]interface{}{
		"active_opportunities": arbMetrics.ValidOpportunities - arbMetrics.TotalExecutions,
		"success_rate":         arbMetrics.SuccessRate,
		"total_profit":         arbMetrics.TotalProfit,
	}
	
	// Broadcast update
	select {
	case d.broadcast <- update:
	default:
		// Channel full, skip
	}
}

// handleBroadcasts sends updates to connected clients
func (d *Dashboard) handleBroadcasts() {
	defer d.wg.Done()
	
	for {
		select {
		case <-d.ctx.Done():
			return
		case update := <-d.broadcast:
			d.broadcastUpdate(update)
		}
	}
}

// broadcastUpdate sends update to all connected clients
func (d *Dashboard) broadcastUpdate(update *DashboardUpdate) {
	data, err := json.Marshal(update)
	if err != nil {
		d.logger.Error("Failed to marshal update", zap.Error(err))
		return
	}
	
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	for _, conn := range d.connections {
		// Apply filters
		if !d.shouldSendUpdate(conn, update) {
			continue
		}
		
		select {
		case conn.send <- data:
		default:
			// Connection send buffer full, close it
			close(conn.send)
			delete(d.connections, conn.ID)
		}
	}
}

// shouldSendUpdate checks if update should be sent to connection
func (d *Dashboard) shouldSendUpdate(conn *DashboardConnection, update *DashboardUpdate) bool {
	// If no filters, send everything
	if len(conn.filters.Accounts) == 0 && 
	   len(conn.filters.Strategies) == 0 && 
	   len(conn.filters.Symbols) == 0 {
		return true
	}
	
	// Apply filters based on update type
	// This is simplified - in production would be more sophisticated
	return true
}

// sendJSON sends JSON response
func (d *Dashboard) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		d.logger.Error("Failed to encode JSON response", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Stop gracefully stops the dashboard
func (d *Dashboard) Stop() error {
	d.cancel()
	
	// Close all WebSocket connections
	d.mu.Lock()
	for _, conn := range d.connections {
		close(conn.send)
		conn.conn.Close()
	}
	d.mu.Unlock()
	
	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := d.server.Shutdown(ctx); err != nil {
		d.logger.Error("Failed to shutdown dashboard server", zap.Error(err))
		return err
	}
	
	d.wg.Wait()
	
	d.logger.Info("Dashboard stopped")
	return nil
}

// GetDashboardHTML returns the dashboard HTML template
func GetDashboardHTML() string {
	return dashboardHTML
}

// Dashboard HTML template (simplified)
const dashboardHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>mExOms Monitoring Dashboard</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1400px; margin: 0 auto; }
        .header { background: #2c3e50; color: white; padding: 20px; border-radius: 5px; margin-bottom: 20px; }
        .metrics-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; }
        .metric-card { background: white; padding: 20px; border-radius: 5px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .metric-value { font-size: 2em; font-weight: bold; color: #3498db; }
        .metric-label { color: #7f8c8d; margin-bottom: 10px; }
        .status { display: inline-block; padding: 5px 10px; border-radius: 3px; }
        .status.active { background: #2ecc71; color: white; }
        .status.inactive { background: #e74c3c; color: white; }
        .chart-container { height: 300px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>mExOms Monitoring Dashboard</h1>
            <p>Real-time system monitoring and performance metrics</p>
        </div>
        
        <div class="metrics-grid">
            <div class="metric-card">
                <div class="metric-label">Orders Per Second</div>
                <div class="metric-value" id="orders-per-sec">0</div>
            </div>
            
            <div class="metric-card">
                <div class="metric-label">Active Accounts</div>
                <div class="metric-value" id="active-accounts">0</div>
            </div>
            
            <div class="metric-card">
                <div class="metric-label">Total P&L</div>
                <div class="metric-value" id="total-pnl">$0.00</div>
            </div>
            
            <div class="metric-card">
                <div class="metric-label">Arbitrage Success Rate</div>
                <div class="metric-value" id="arb-success">0%</div>
            </div>
        </div>
        
        <div class="metric-card" style="margin-top: 20px;">
            <h3>System Performance</h3>
            <div class="chart-container" id="performance-chart"></div>
        </div>
        
        <div class="metric-card" style="margin-top: 20px;">
            <h3>Strategy Performance</h3>
            <div id="strategy-table"></div>
        </div>
    </div>
    
    <script>
        // WebSocket connection
        const ws = new WebSocket('ws://localhost:8080/ws');
        
        ws.onopen = function() {
            console.log('Connected to dashboard');
            ws.send(JSON.stringify({ type: 'subscribe', filters: {} }));
        };
        
        ws.onmessage = function(event) {
            const update = JSON.parse(event.data);
            if (update.type === 'metrics') {
                updateMetrics(update.data);
            }
        };
        
        function updateMetrics(data) {
            document.getElementById('orders-per-sec').textContent = data.orders_per_second || 0;
            document.getElementById('active-accounts').textContent = data.active_accounts || 0;
            
            // Update other metrics...
        }
    </script>
</body>
</html>
`