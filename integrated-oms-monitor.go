package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
)

type SystemMonitor struct {
	mu              sync.RWMutex
	Services        map[string]ServiceInfo
	MarketData      map[string]MarketTicker
	NatsStats       NatsInfo
	OrderStats      OrderInfo
	WebSocketStats  WSInfo
}

type ServiceInfo struct {
	Name    string
	Status  string
	PID     string
	Uptime  string
	CPU     string
	Memory  string
}

type MarketTicker struct {
	Symbol    string  `json:"s"`
	Price     string  `json:"c"`
	Change    string  `json:"p"`
	ChangePct string  `json:"P"`
	Volume    string  `json:"v"`
	High      string  `json:"h"`
	Low       string  `json:"l"`
	Updated   time.Time
}

type NatsInfo struct {
	Connected bool
	Messages  int64
	Subjects  int
}

type OrderInfo struct {
	Total   int64
	Success int64
	Failed  int64
	Pending int64
}

type WSInfo struct {
	SpotConnected    bool
	FuturesConnected bool
	SpotMessages     int64
	FuturesMessages  int64
}

var monitor = &SystemMonitor{
	Services:   make(map[string]ServiceInfo),
	MarketData: make(map[string]MarketTicker),
}

func checkServices() {
	services := []struct {
		name    string
		process string
	}{
		{"OMS Core", "oms-core"},
		{"OMS Server", "oms-server"},
		{"Binance Spot", "binance-spot"},
		{"Binance Futures", "binance-futures"},
		{"NATS", "nats-server"},
		{"Vault", "vault"},
	}

	for _, svc := range services {
		cmd := exec.Command("bash", "-c", fmt.Sprintf("ps aux | grep '%s' | grep -v grep | head -1", svc.process))
		output, _ := cmd.Output()
		
		monitor.mu.Lock()
		if len(output) > 0 {
			fields := strings.Fields(string(output))
			if len(fields) > 10 {
				pid := fields[1]
				cpu := fields[2]
				mem := fields[3]
				
				// Get uptime
				uptimeCmd := exec.Command("ps", "-o", "etime=", "-p", pid)
				uptime, _ := uptimeCmd.Output()
				
				monitor.Services[svc.name] = ServiceInfo{
					Name:   svc.name,
					Status: "RUNNING",
					PID:    pid,
					CPU:    cpu + "%",
					Memory: mem + "%",
					Uptime: strings.TrimSpace(string(uptime)),
				}
			}
		} else {
			monitor.Services[svc.name] = ServiceInfo{
				Name:   svc.name,
				Status: "STOPPED",
			}
		}
		monitor.mu.Unlock()
	}
}

func connectBinanceWS() {
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt"}
	streams := []string{}
	for _, s := range symbols {
		streams = append(streams, s+"@ticker")
	}
	
	url := fmt.Sprintf("wss://stream.binance.com:9443/stream?streams=%s", strings.Join(streams, "/"))
	
	for {
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("WebSocket dial error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		
		monitor.mu.Lock()
		monitor.WebSocketStats.SpotConnected = true
		monitor.mu.Unlock()
		
		for {
			var msg struct {
				Stream string          `json:"stream"`
				Data   json.RawMessage `json:"data"`
			}
			
			err := c.ReadJSON(&msg)
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				break
			}
			
			var ticker MarketTicker
			if err := json.Unmarshal(msg.Data, &ticker); err == nil {
				ticker.Updated = time.Now()
				
				monitor.mu.Lock()
				monitor.MarketData[ticker.Symbol] = ticker
				monitor.WebSocketStats.SpotMessages++
				monitor.mu.Unlock()
			}
		}
		
		c.Close()
		monitor.mu.Lock()
		monitor.WebSocketStats.SpotConnected = false
		monitor.mu.Unlock()
		
		time.Sleep(5 * time.Second)
	}
}

func connectNATS() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Printf("NATS connection error: %v", err)
		return
	}
	defer nc.Close()
	
	monitor.mu.Lock()
	monitor.NatsStats.Connected = true
	monitor.mu.Unlock()
	
	// Subscribe to all messages
	nc.Subscribe(">", func(msg *nats.Msg) {
		monitor.mu.Lock()
		monitor.NatsStats.Messages++
		monitor.mu.Unlock()
	})
	
	// Keep connection alive
	for {
		time.Sleep(1 * time.Second)
		stats := nc.Stats()
		monitor.mu.Lock()
		monitor.NatsStats.Subjects = len(stats.Subs)
		monitor.mu.Unlock()
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
    <title>OMS Integrated Monitor</title>
    <meta charset="utf-8">
    <meta http-equiv="refresh" content="5">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: #0a0f1b;
            color: #e0e6ed;
            padding: 20px;
        }
        .container { max-width: 1800px; margin: 0 auto; }
        h1 {
            text-align: center;
            color: #4a9eff;
            margin-bottom: 30px;
            font-size: 32px;
            font-weight: 300;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
        }
        .card {
            background: #1a2332;
            border: 1px solid #2a3441;
            border-radius: 8px;
            padding: 20px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.2);
        }
        .card h2 {
            color: #ffa500;
            margin-bottom: 15px;
            font-size: 18px;
            font-weight: 500;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th {
            text-align: left;
            padding: 10px;
            border-bottom: 2px solid #2a3441;
            color: #8b98a8;
            font-weight: 600;
            font-size: 14px;
        }
        td {
            padding: 10px;
            border-bottom: 1px solid #2a3441;
            font-size: 14px;
        }
        .status-running { color: #4ade80; }
        .status-stopped { color: #f87171; }
        .price-up { color: #4ade80; }
        .price-down { color: #f87171; }
        .metric {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid #2a3441;
        }
        .metric:last-child { border-bottom: none; }
        .metric-label { color: #8b98a8; }
        .metric-value {
            font-weight: 600;
            color: #4a9eff;
            font-size: 16px;
        }
        .live-indicator {
            display: inline-block;
            width: 8px;
            height: 8px;
            background: #4ade80;
            border-radius: 50%;
            margin-right: 8px;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .timestamp {
            text-align: center;
            color: #8b98a8;
            margin-top: 30px;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1><span class="live-indicator"></span>OMS Integrated Real-Time Monitor</h1>
        
        <div class="grid">
            <div class="card">
                <h2>📡 System Services</h2>
                <table>
                    <tr>
                        <th>Service</th>
                        <th>Status</th>
                        <th>PID</th>
                        <th>CPU</th>
                        <th>Memory</th>
                        <th>Uptime</th>
                    </tr>
                    {{range $name, $svc := .Services}}
                    <tr>
                        <td>{{$svc.Name}}</td>
                        <td class="{{if eq $svc.Status "RUNNING"}}status-running{{else}}status-stopped{{end}}">
                            ● {{$svc.Status}}
                        </td>
                        <td>{{if $svc.PID}}{{$svc.PID}}{{else}}-{{end}}</td>
                        <td>{{if $svc.CPU}}{{$svc.CPU}}{{else}}-{{end}}</td>
                        <td>{{if $svc.Memory}}{{$svc.Memory}}{{else}}-{{end}}</td>
                        <td>{{if $svc.Uptime}}{{$svc.Uptime}}{{else}}-{{end}}</td>
                    </tr>
                    {{end}}
                </table>
            </div>
            
            <div class="card">
                <h2>📊 Live Market Data</h2>
                <table>
                    <tr>
                        <th>Symbol</th>
                        <th>Price</th>
                        <th>24h Change</th>
                        <th>Volume</th>
                        <th>High/Low</th>
                    </tr>
                    {{range $symbol, $ticker := .MarketData}}
                    <tr>
                        <td style="font-weight: bold;">{{$ticker.Symbol}}</td>
                        <td>${{$ticker.Price}}</td>
                        <td class="{{if (gt $ticker.ChangePct "0")}}price-up{{else}}price-down{{end}}">
                            {{$ticker.ChangePct}}%
                        </td>
                        <td>{{$ticker.Volume}}</td>
                        <td>${{$ticker.High}} / ${{$ticker.Low}}</td>
                    </tr>
                    {{end}}
                </table>
            </div>
            
            <div class="card">
                <h2>🔌 Connection Status</h2>
                <div class="metric">
                    <span class="metric-label">NATS Message Broker</span>
                    <span class="metric-value {{if .NatsStats.Connected}}status-running{{else}}status-stopped{{end}}">
                        {{if .NatsStats.Connected}}● CONNECTED{{else}}● DISCONNECTED{{end}}
                    </span>
                </div>
                <div class="metric">
                    <span class="metric-label">NATS Messages</span>
                    <span class="metric-value">{{.NatsStats.Messages}}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Binance Spot WS</span>
                    <span class="metric-value {{if .WebSocketStats.SpotConnected}}status-running{{else}}status-stopped{{end}}">
                        {{if .WebSocketStats.SpotConnected}}● CONNECTED{{else}}● DISCONNECTED{{end}}
                    </span>
                </div>
                <div class="metric">
                    <span class="metric-label">Spot Messages</span>
                    <span class="metric-value">{{.WebSocketStats.SpotMessages}}</span>
                </div>
            </div>
            
            <div class="card">
                <h2>📈 Order Statistics</h2>
                <div class="metric">
                    <span class="metric-label">Total Orders</span>
                    <span class="metric-value">{{.OrderStats.Total}}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Successful</span>
                    <span class="metric-value" style="color: #4ade80;">{{.OrderStats.Success}}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Failed</span>
                    <span class="metric-value" style="color: #f87171;">{{.OrderStats.Failed}}</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Pending</span>
                    <span class="metric-value" style="color: #fbbf24;">{{.OrderStats.Pending}}</span>
                </div>
            </div>
        </div>
        
        <div class="timestamp">
            Last Updated: {{.Timestamp}}
        </div>
    </div>
</body>
</html>`

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	monitor.mu.RLock()
	data := struct {
		Services       map[string]ServiceInfo
		MarketData     map[string]MarketTicker
		NatsStats      NatsInfo
		OrderStats     OrderInfo
		WebSocketStats WSInfo
		Timestamp      string
	}{
		Services:       monitor.Services,
		MarketData:     monitor.MarketData,
		NatsStats:      monitor.NatsStats,
		OrderStats:     monitor.OrderStats,
		WebSocketStats: monitor.WebSocketStats,
		Timestamp:      time.Now().Format("2006-01-02 15:04:05"),
	}
	monitor.mu.RUnlock()
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, strings.ReplaceAll(strings.ReplaceAll(dashboardHTML, 
		"{{range $name, $svc := .Services}}", ""),
		"{{range $symbol, $ticker := .MarketData}}", ""))
}

func main() {
	// Start monitoring goroutines
	go func() {
		for {
			checkServices()
			time.Sleep(5 * time.Second)
		}
	}()
	
	go connectBinanceWS()
	go connectNATS()
	
	// HTTP server
	http.HandleFunc("/", handleDashboard)
	
	fmt.Println("🚀 OMS Integrated Monitor running at http://localhost:8081")
	fmt.Println("📊 Displaying real-time system status, market data, and metrics")
	log.Fatal(http.ListenAndServe(":8081", nil))
}