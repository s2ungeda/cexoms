package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type SystemStatus struct {
	mu                  sync.RWMutex
	Services            map[string]ServiceStatus
	WebSocketStatus     map[string]WSStatus
	OrderStats          OrderStatistics
	MarketDataStats     MarketDataStatistics
	SystemHealth        HealthStatus
	LastUpdate          time.Time
}

type ServiceStatus struct {
	Name        string
	Status      string // running, stopped, error
	PID         string
	Uptime      string
	LastChecked time.Time
}

type WSStatus struct {
	Exchange   string
	Market     string
	Connected  bool
	LastPing   time.Time
	Messages   int64
}

type OrderStatistics struct {
	TotalOrders      int64
	OrdersPerMinute  float64
	SuccessfulOrders int64
	FailedOrders     int64
	PendingOrders    int64
	AverageLatency   float64
}

type MarketDataStatistics struct {
	TickersReceived   int64
	TradesReceived    int64
	OrderBooksUpdated int64
	MessagesPerSecond float64
	DataLatency       float64
}

type HealthStatus struct {
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	NetworkIO   string
}

var status = &SystemStatus{
	Services:        make(map[string]ServiceStatus),
	WebSocketStatus: make(map[string]WSStatus),
}

const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
    <title>OMS System Monitor</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0d1117;
            color: #c9d1d9;
            padding: 20px;
        }
        .container { max-width: 1600px; margin: 0 auto; }
        h1 {
            color: #58a6ff;
            text-align: center;
            margin-bottom: 30px;
            font-size: 32px;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 20px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.12);
        }
        .card h2 {
            color: #f0883e;
            margin-bottom: 15px;
            font-size: 18px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th {
            text-align: left;
            padding: 8px;
            border-bottom: 1px solid #30363d;
            color: #58a6ff;
            font-weight: 600;
        }
        td {
            padding: 8px;
            border-bottom: 1px solid #21262d;
        }
        .status-running { color: #3fb950; }
        .status-stopped { color: #f85149; }
        .status-error { color: #d29922; }
        .metric {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid #21262d;
        }
        .metric:last-child { border-bottom: none; }
        .metric-value {
            font-weight: bold;
            color: #58a6ff;
        }
        .ws-connected { color: #3fb950; }
        .ws-disconnected { color: #f85149; }
        .timestamp {
            text-align: center;
            color: #8b949e;
            margin-top: 20px;
            font-size: 14px;
        }
        .icon {
            width: 20px;
            height: 20px;
            display: inline-block;
        }
        .refresh-indicator {
            animation: spin 2s linear infinite;
        }
        @keyframes spin {
            100% { transform: rotate(360deg); }
        }
    </style>
    <script>
        function updateDashboard() {
            fetch('/api/status')
                .then(response => response.json())
                .then(data => {
                    // Update services
                    let servicesHTML = '<table><tr><th>Service</th><th>Status</th><th>PID</th><th>Uptime</th></tr>';
                    for (const [name, service] of Object.entries(data.services)) {
                        const statusClass = 'status-' + service.status;
                        servicesHTML += '<tr>' +
                            '<td>' + service.name + '</td>' +
                            '<td class="' + statusClass + '">● ' + service.status.toUpperCase() + '</td>' +
                            '<td>' + (service.pid || '-') + '</td>' +
                            '<td>' + (service.uptime || '-') + '</td>' +
                            '</tr>';
                    }
                    servicesHTML += '</table>';
                    document.getElementById('services').innerHTML = servicesHTML;

                    // Update WebSocket connections
                    let wsHTML = '<table><tr><th>Exchange</th><th>Market</th><th>Status</th><th>Messages</th></tr>';
                    for (const [key, ws] of Object.entries(data.websockets)) {
                        const statusClass = ws.connected ? 'ws-connected' : 'ws-disconnected';
                        const status = ws.connected ? '● CONNECTED' : '● DISCONNECTED';
                        wsHTML += '<tr>' +
                            '<td>' + ws.exchange + '</td>' +
                            '<td>' + ws.market + '</td>' +
                            '<td class="' + statusClass + '">' + status + '</td>' +
                            '<td>' + ws.messages.toLocaleString() + '</td>' +
                            '</tr>';
                    }
                    wsHTML += '</table>';
                    document.getElementById('websockets').innerHTML = wsHTML;

                    // Update order statistics
                    let orderHTML = '';
                    orderHTML += '<div class="metric"><span>Total Orders</span><span class="metric-value">' + data.orderStats.totalOrders.toLocaleString() + '</span></div>';
                    orderHTML += '<div class="metric"><span>Orders/Minute</span><span class="metric-value">' + data.orderStats.ordersPerMinute.toFixed(2) + '</span></div>';
                    orderHTML += '<div class="metric"><span>Success Rate</span><span class="metric-value">' + 
                        ((data.orderStats.successfulOrders / Math.max(1, data.orderStats.totalOrders) * 100).toFixed(1)) + '%</span></div>';
                    orderHTML += '<div class="metric"><span>Avg Latency</span><span class="metric-value">' + data.orderStats.avgLatency.toFixed(1) + ' ms</span></div>';
                    orderHTML += '<div class="metric"><span>Pending Orders</span><span class="metric-value">' + data.orderStats.pendingOrders + '</span></div>';
                    document.getElementById('order-stats').innerHTML = orderHTML;

                    // Update market data statistics
                    let marketHTML = '';
                    marketHTML += '<div class="metric"><span>Tickers Received</span><span class="metric-value">' + data.marketStats.tickersReceived.toLocaleString() + '</span></div>';
                    marketHTML += '<div class="metric"><span>Trades Received</span><span class="metric-value">' + data.marketStats.tradesReceived.toLocaleString() + '</span></div>';
                    marketHTML += '<div class="metric"><span>OrderBooks Updated</span><span class="metric-value">' + data.marketStats.orderBooksUpdated.toLocaleString() + '</span></div>';
                    marketHTML += '<div class="metric"><span>Messages/Second</span><span class="metric-value">' + data.marketStats.messagesPerSecond.toFixed(1) + '</span></div>';
                    marketHTML += '<div class="metric"><span>Data Latency</span><span class="metric-value">' + data.marketStats.dataLatency.toFixed(1) + ' ms</span></div>';
                    document.getElementById('market-stats').innerHTML = marketHTML;

                    // Update system health
                    let healthHTML = '';
                    healthHTML += '<div class="metric"><span>CPU Usage</span><span class="metric-value">' + data.systemHealth.cpuUsage.toFixed(1) + '%</span></div>';
                    healthHTML += '<div class="metric"><span>Memory Usage</span><span class="metric-value">' + data.systemHealth.memoryUsage.toFixed(1) + '%</span></div>';
                    healthHTML += '<div class="metric"><span>Disk Usage</span><span class="metric-value">' + data.systemHealth.diskUsage.toFixed(1) + '%</span></div>';
                    healthHTML += '<div class="metric"><span>Network I/O</span><span class="metric-value">' + data.systemHealth.networkIO + '</span></div>';
                    document.getElementById('system-health').innerHTML = healthHTML;

                    // Update timestamp
                    document.getElementById('timestamp').textContent = 'Last Update: ' + new Date(data.lastUpdate).toLocaleString();
                });
        }

        setInterval(updateDashboard, 1000);
        window.onload = updateDashboard;
    </script>
</head>
<body>
    <div class="container">
        <h1>🔧 OMS System Monitor</h1>
        
        <div class="grid">
            <div class="card">
                <h2>🖥️ Services Status</h2>
                <div id="services">Loading...</div>
            </div>
            
            <div class="card">
                <h2>🔌 WebSocket Connections</h2>
                <div id="websockets">Loading...</div>
            </div>
            
            <div class="card">
                <h2>📊 Order Statistics</h2>
                <div id="order-stats">Loading...</div>
            </div>
            
            <div class="card">
                <h2>📈 Market Data Flow</h2>
                <div id="market-stats">Loading...</div>
            </div>
            
            <div class="card">
                <h2>💻 System Health</h2>
                <div id="system-health">Loading...</div>
            </div>
        </div>
        
        <div class="timestamp" id="timestamp"></div>
    </div>
</body>
</html>`

func checkServices() {
	services := []string{
		"oms-core",
		"oms-server", 
		"binance-spot",
		"binance-futures",
		"nats",
		"vault",
	}

	for _, service := range services {
		cmd := exec.Command("bash", "-c", fmt.Sprintf("ps aux | grep -E '(%s|%s/main.go)' | grep -v grep | head -1", service, service))
		output, _ := cmd.Output()
		
		status.mu.Lock()
		if len(output) > 0 {
			fields := strings.Fields(string(output))
			if len(fields) > 1 {
				status.Services[service] = ServiceStatus{
					Name:        service,
					Status:      "running",
					PID:         fields[1],
					Uptime:      getProcessUptime(fields[1]),
					LastChecked: time.Now(),
				}
			}
		} else {
			status.Services[service] = ServiceStatus{
				Name:        service,
				Status:      "stopped",
				LastChecked: time.Now(),
			}
		}
		status.mu.Unlock()
	}
}

func getProcessUptime(pid string) string {
	cmd := exec.Command("ps", "-o", "etime=", "-p", pid)
	output, err := cmd.Output()
	if err != nil {
		return "N/A"
	}
	return strings.TrimSpace(string(output))
}

func checkWebSocketConnections() {
	// Check logs for WebSocket activity
	status.mu.Lock()
	defer status.mu.Unlock()

	// Binance Spot WebSocket
	spotLog, _ := ioutil.ReadFile("logs/binance-spot.log")
	spotLines := strings.Split(string(spotLog), "\n")
	spotConnected := false
	for i := len(spotLines) - 1; i >= 0 && i > len(spotLines)-10; i-- {
		if strings.Contains(spotLines[i], "WebSocket connected") || strings.Contains(spotLines[i], "heartbeat") {
			spotConnected = true
			break
		}
	}
	
	status.WebSocketStatus["binance-spot"] = WSStatus{
		Exchange:  "Binance",
		Market:    "Spot",
		Connected: spotConnected,
		LastPing:  time.Now(),
		Messages:  int64(strings.Count(string(spotLog), "message received")),
	}

	// Binance Futures WebSocket
	futuresLog, _ := ioutil.ReadFile("logs/binance-futures.log")
	futuresLines := strings.Split(string(futuresLog), "\n")
	futuresConnected := false
	for i := len(futuresLines) - 1; i >= 0 && i > len(futuresLines)-10; i-- {
		if strings.Contains(futuresLines[i], "WebSocket connected") || strings.Contains(futuresLines[i], "heartbeat") {
			futuresConnected = true
			break
		}
	}
	
	status.WebSocketStatus["binance-futures"] = WSStatus{
		Exchange:  "Binance",
		Market:    "Futures",
		Connected: futuresConnected,
		LastPing:  time.Now(),
		Messages:  int64(strings.Count(string(futuresLog), "message received")),
	}
}

func updateStatistics() {
	status.mu.Lock()
	defer status.mu.Unlock()

	// Parse logs for order statistics
	serverLog, _ := ioutil.ReadFile("logs/oms-server.log")
	logContent := string(serverLog)
	
	status.OrderStats = OrderStatistics{
		TotalOrders:      int64(strings.Count(logContent, "order created")),
		SuccessfulOrders: int64(strings.Count(logContent, "order filled")),
		FailedOrders:     int64(strings.Count(logContent, "order failed")),
		PendingOrders:    int64(strings.Count(logContent, "order pending")),
		OrdersPerMinute:  calculateOrdersPerMinute(logContent),
		AverageLatency:   45.3, // Mock for now
	}

	// Market data statistics
	status.MarketDataStats = MarketDataStatistics{
		TickersReceived:   int64(strings.Count(logContent, "ticker update")),
		TradesReceived:    int64(strings.Count(logContent, "trade update")),
		OrderBooksUpdated: int64(strings.Count(logContent, "orderbook update")),
		MessagesPerSecond: 150.5, // Mock for now
		DataLatency:       12.8,  // Mock for now
	}

	// System health
	status.SystemHealth = getSystemHealth()
}

func calculateOrdersPerMinute(logContent string) float64 {
	// Simple calculation based on last hour
	lines := strings.Split(logContent, "\n")
	recentOrders := 0
	// cutoffTime := time.Now().Add(-1 * time.Hour)
	
	for _, line := range lines {
		if strings.Contains(line, "order created") {
			// Try to parse timestamp
			parts := strings.Fields(line)
			if len(parts) > 2 {
				// Simplified - just count recent orders
				recentOrders++
			}
		}
	}
	
	return float64(recentOrders) / 60.0
}

func getSystemHealth() HealthStatus {
	health := HealthStatus{}

	// CPU usage
	cmd := exec.Command("bash", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2}' | cut -d'%' -f1")
	output, _ := cmd.Output()
	fmt.Sscanf(string(output), "%f", &health.CPUUsage)

	// Memory usage
	cmd = exec.Command("bash", "-c", "free | grep Mem | awk '{print ($3/$2) * 100.0}'")
	output, _ = cmd.Output()
	fmt.Sscanf(string(output), "%f", &health.MemoryUsage)

	// Disk usage
	cmd = exec.Command("bash", "-c", "df -h / | tail -1 | awk '{print $5}' | sed 's/%//'")
	output, _ = cmd.Output()
	fmt.Sscanf(string(output), "%f", &health.DiskUsage)

	// Network I/O (simplified)
	health.NetworkIO = "125 KB/s"

	return health
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(dashboardHTML))
}

func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	status.mu.RLock()
	defer status.mu.RUnlock()

	response := map[string]interface{}{
		"services":     status.Services,
		"websockets":   status.WebSocketStatus,
		"orderStats":   status.OrderStats,
		"marketStats":  status.MarketDataStats,
		"systemHealth": status.SystemHealth,
		"lastUpdate":   status.LastUpdate,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func monitoringLoop() {
	for {
		checkServices()
		checkWebSocketConnections()
		updateStatistics()
		
		status.mu.Lock()
		status.LastUpdate = time.Now()
		status.mu.Unlock()
		
		time.Sleep(2 * time.Second)
	}
}

func main() {
	// Create logs directory if it doesn't exist
	os.MkdirAll("logs", 0755)

	// Start monitoring loop
	go monitoringLoop()

	// HTTP handlers
	http.HandleFunc("/", handleDashboard)
	http.HandleFunc("/api/status", handleAPIStatus)

	fmt.Println("OMS System Monitor running at http://localhost:8081")
	fmt.Println("Monitoring OMS services, WebSocket connections, and system health...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}