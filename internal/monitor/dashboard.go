package monitor

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/mExOms/oms/internal/position"
	"github.com/mExOms/oms/internal/risk"
)

// DashboardServer provides a web-based monitoring dashboard
type DashboardServer struct {
	mu sync.RWMutex
	
	// Dependencies
	metrics         *MetricsCollector
	health          *HealthChecker
	logger          *Logger
	positionManager *position.PositionManager
	riskEngine      *risk.RiskEngine
	
	// Server configuration
	addr string
	
	// Real-time data
	realtimeData map[string]interface{}
	wsClients    map[*wsClient]bool
}

// wsClient represents a WebSocket client
type wsClient struct {
	conn   interface{} // In production, use gorilla/websocket
	send   chan []byte
}

// NewDashboardServer creates a new dashboard server
func NewDashboardServer(addr string, deps DashboardDeps) *DashboardServer {
	return &DashboardServer{
		addr:            addr,
		metrics:         deps.Metrics,
		health:          deps.Health,
		logger:          deps.Logger,
		positionManager: deps.PositionManager,
		riskEngine:      deps.RiskEngine,
		realtimeData:    make(map[string]interface{}),
		wsClients:       make(map[*wsClient]bool),
	}
}

// DashboardDeps holds dashboard dependencies
type DashboardDeps struct {
	Metrics         *MetricsCollector
	Health          *HealthChecker
	Logger          *Logger
	PositionManager *position.PositionManager
	RiskEngine      *risk.RiskEngine
}

// Start starts the dashboard server
func (ds *DashboardServer) Start() error {
	// Setup routes
	mux := http.NewServeMux()
	
	// Static pages
	mux.HandleFunc("/", ds.handleIndex)
	mux.HandleFunc("/health", ds.health.HTTPHandler())
	
	// API endpoints
	mux.HandleFunc("/api/metrics", ds.handleMetrics)
	mux.HandleFunc("/api/positions", ds.handlePositions)
	mux.HandleFunc("/api/risk", ds.handleRisk)
	mux.HandleFunc("/api/logs", ds.handleLogs)
	mux.HandleFunc("/api/system", ds.handleSystem)
	
	// WebSocket endpoint (simplified for demo)
	mux.HandleFunc("/ws", ds.handleWebSocket)
	
	// Start data updater
	go ds.updateRealtimeData()
	
	ds.logger.Info("Starting dashboard server", map[string]interface{}{
		"address": ds.addr,
	})
	
	return http.ListenAndServe(ds.addr, mux)
}

// handleIndex serves the main dashboard page
func (ds *DashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>OMS Monitoring Dashboard</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: #333; color: white; padding: 20px; margin: -20px -20px 20px; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; }
        .card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .card h3 { margin-top: 0; color: #333; }
        .metric { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #eee; }
        .metric:last-child { border-bottom: none; }
        .value { font-weight: bold; color: #2196F3; }
        .status { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; }
        .status.healthy { background: #4CAF50; color: white; }
        .status.degraded { background: #FF9800; color: white; }
        .status.unhealthy { background: #F44336; color: white; }
        .chart { height: 200px; background: #fafafa; border-radius: 4px; display: flex; align-items: center; justify-content: center; color: #999; }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>OMS Monitoring Dashboard</h1>
            <p>Real-time monitoring and metrics</p>
        </div>
    </div>
    
    <div class="container">
        <div class="metrics">
            <!-- System Health -->
            <div class="card">
                <h3>System Health</h3>
                <div id="health-status"></div>
            </div>
            
            <!-- Positions -->
            <div class="card">
                <h3>Position Summary</h3>
                <div id="position-summary"></div>
            </div>
            
            <!-- Risk Metrics -->
            <div class="card">
                <h3>Risk Metrics</h3>
                <div id="risk-metrics"></div>
            </div>
            
            <!-- Performance -->
            <div class="card">
                <h3>Performance</h3>
                <div id="performance-metrics"></div>
            </div>
            
            <!-- Order Flow -->
            <div class="card">
                <h3>Order Flow</h3>
                <div class="chart">Real-time order chart</div>
            </div>
            
            <!-- Recent Logs -->
            <div class="card">
                <h3>Recent Activity</h3>
                <div id="recent-logs" style="max-height: 300px; overflow-y: auto;"></div>
            </div>
        </div>
    </div>
    
    <script>
        // Auto-refresh data
        function updateDashboard() {
            // Fetch health
            fetch('/health')
                .then(r => r.json())
                .then(data => {
                    const healthDiv = document.getElementById('health-status');
                    healthDiv.innerHTML = data.components.map(c => 
                        '<div class="metric">' +
                        '<span>' + c.name + '</span>' +
                        '<span class="status ' + c.status + '">' + c.status + '</span>' +
                        '</div>'
                    ).join('');
                });
            
            // Fetch positions
            fetch('/api/positions')
                .then(r => r.json())
                .then(data => {
                    const posDiv = document.getElementById('position-summary');
                    posDiv.innerHTML = 
                        '<div class="metric"><span>Total Positions</span><span class="value">' + data.count + '</span></div>' +
                        '<div class="metric"><span>Total Value</span><span class="value">$' + data.total_value + '</span></div>' +
                        '<div class="metric"><span>Unrealized P&L</span><span class="value">$' + data.unrealized_pnl + '</span></div>';
                });
            
            // Fetch risk metrics
            fetch('/api/risk')
                .then(r => r.json())
                .then(data => {
                    const riskDiv = document.getElementById('risk-metrics');
                    riskDiv.innerHTML = 
                        '<div class="metric"><span>Max Leverage</span><span class="value">' + data.max_leverage + 'x</span></div>' +
                        '<div class="metric"><span>Total Exposure</span><span class="value">$' + data.total_exposure + '</span></div>' +
                        '<div class="metric"><span>Daily P&L</span><span class="value">$' + data.daily_pnl + '</span></div>';
                });
            
            // Fetch performance metrics
            fetch('/api/metrics')
                .then(r => r.json())
                .then(data => {
                    const perfDiv = document.getElementById('performance-metrics');
                    perfDiv.innerHTML = 
                        '<div class="metric"><span>Orders/sec</span><span class="value">' + data.orders_per_second + '</span></div>' +
                        '<div class="metric"><span>Avg Latency</span><span class="value">' + data.avg_latency_ms + 'ms</span></div>' +
                        '<div class="metric"><span>Memory Usage</span><span class="value">' + data.memory_mb + 'MB</span></div>';
                });
        }
        
        // Update every 2 seconds
        updateDashboard();
        setInterval(updateDashboard, 2000);
        
        // WebSocket for real-time updates (simplified)
        // In production, implement proper WebSocket handling
    </script>
</body>
</html>
`
	
	t, _ := template.New("dashboard").Parse(tmpl)
	t.Execute(w, nil)
}

// API handlers

func (ds *DashboardServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ds.metrics.GetMetrics()
	
	// Calculate derived metrics
	response := map[string]interface{}{
		"orders_per_second": 1250,  // Mock value
		"avg_latency_ms":    2.3,   // Mock value
		"memory_mb":         2048,  // Mock value
		"raw_metrics":       metrics,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ds *DashboardServer) handlePositions(w http.ResponseWriter, r *http.Request) {
	positions := ds.positionManager.GetAllPositions()
	unrealizedPnL, _ := ds.positionManager.CalculateTotalPnL()
	
	response := map[string]interface{}{
		"count":          len(positions),
		"total_value":    "125000.50", // Mock value
		"unrealized_pnl": unrealizedPnL.String(),
		"positions":      positions,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ds *DashboardServer) handleRisk(w http.ResponseWriter, r *http.Request) {
	metrics := ds.riskEngine.GetMetrics()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (ds *DashboardServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	// Return recent logs (mock for demo)
	logs := []map[string]interface{}{
		{
			"timestamp": time.Now().Add(-5 * time.Second),
			"level":     "INFO",
			"message":   "Order placed successfully",
			"exchange":  "binance",
			"symbol":    "BTCUSDT",
		},
		{
			"timestamp": time.Now().Add(-10 * time.Second),
			"level":     "WARN",
			"message":   "High latency detected",
			"latency":   "250ms",
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (ds *DashboardServer) handleSystem(w http.ResponseWriter, r *http.Request) {
	system := map[string]interface{}{
		"uptime":      time.Since(time.Now().Add(-24 * time.Hour)).String(),
		"version":     "1.0.0",
		"connections": 42,
		"cpu_percent": 15.5,
		"memory_mb":   2048,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(system)
}

func (ds *DashboardServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Simplified WebSocket handler
	// In production, use gorilla/websocket
	fmt.Fprintf(w, "WebSocket endpoint - use a WebSocket client to connect")
}

// updateRealtimeData updates real-time dashboard data
func (ds *DashboardServer) updateRealtimeData() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		// Collect real-time data
		data := map[string]interface{}{
			"timestamp":     time.Now(),
			"orders_second": 1250 + time.Now().Unix()%100,
			"positions":     len(ds.positionManager.GetAllPositions()),
			"active_trades": 15 + time.Now().Unix()%10,
		}
		
		ds.mu.Lock()
		ds.realtimeData = data
		ds.mu.Unlock()
		
		// Broadcast to WebSocket clients
		ds.broadcastUpdate(data)
	}
}

// broadcastUpdate sends updates to all WebSocket clients
func (ds *DashboardServer) broadcastUpdate(data interface{}) {
	// In production, implement proper WebSocket broadcasting
}