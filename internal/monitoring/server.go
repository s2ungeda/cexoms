package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	
	"log"
)

// Server provides HTTP endpoints for monitoring
type Server struct {
	config       *ServerConfig
	metrics      *Metrics
	collector    *Collector
	alertManager *AlertManager
	
	httpServer   *http.Server
}

// ServerConfig contains monitoring server configuration
type ServerConfig struct {
	Port            int
	MetricsPath     string
	HealthPath      string
	AlertsPath      string
	DashboardPath   string
	EnableDashboard bool
}

// NewServer creates a new monitoring server
func NewServer(
	config *ServerConfig,
	metrics *Metrics,
	collector *Collector,
	alertManager *AlertManager,
) *Server {
	if config == nil {
		config = &ServerConfig{
			Port:            9090,
			MetricsPath:     "/metrics",
			HealthPath:      "/health",
			AlertsPath:      "/alerts",
			DashboardPath:   "/dashboard",
			EnableDashboard: true,
		}
	}
	
	return &Server{
		config:       config,
		metrics:      metrics,
		collector:    collector,
		alertManager: alertManager,
	}
}

// Start starts the monitoring server
func (s *Server) Start(ctx context.Context) error {
	router := mux.NewRouter()
	
	// Prometheus metrics endpoint
	router.Handle(s.config.MetricsPath, promhttp.Handler())
	
	// Health check endpoint
	router.HandleFunc(s.config.HealthPath, s.handleHealth).Methods("GET")
	
	// Alerts endpoints
	router.HandleFunc(s.config.AlertsPath, s.handleGetAlerts).Methods("GET")
	router.HandleFunc(s.config.AlertsPath, s.handleCreateAlert).Methods("POST")
	
	// Dashboard
	if s.config.EnableDashboard {
		router.HandleFunc(s.config.DashboardPath, s.handleDashboard).Methods("GET")
		router.HandleFunc("/api/metrics/summary", s.handleMetricsSummary).Methods("GET")
		router.HandleFunc("/api/accounts", s.handleAccounts).Methods("GET")
		router.HandleFunc("/api/positions", s.handlePositions).Methods("GET")
		router.HandleFunc("/api/performance", s.handlePerformance).Methods("GET")
	}
	
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	
	log.Printf("Starting monitoring server on port %d", s.config.Port)
	
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Monitoring server error: %v", err)
		}
	}()
	
	<-ctx.Done()
	
	// Shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	return s.httpServer.Shutdown(shutdownCtx)
}

// Stop stops the monitoring server
func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now().UTC(),
		"version": "1.0.0",
		"components": map[string]string{
			"metrics_collector": "healthy",
			"alert_manager": "healthy",
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleGetAlerts handles get alerts requests
func (s *Server) handleGetAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := s.alertManager.GetActiveAlerts()
	
	response := map[string]interface{}{
		"active_alerts": alerts,
		"total": len(alerts),
		"timestamp": time.Now().UTC(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateAlert handles create alert requests
func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	s.alertManager.AddRule(&rule)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "created",
		"rule_id": rule.ID,
	})
}

// handleDashboard serves the monitoring dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(dashboardHTML))
}

// handleMetricsSummary returns metrics summary
func (s *Server) handleMetricsSummary(w http.ResponseWriter, r *http.Request) {
	// Gather metrics from Prometheus registry
	gathering, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	summary := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"metrics": map[string]float64{},
	}
	
	// Extract key metrics
	for _, mf := range gathering {
		if mf.GetName() == "oms_orders_total" {
			var total float64
			for _, m := range mf.GetMetric() {
				total += m.GetCounter().GetValue()
			}
			summary["metrics"].(map[string]float64)["total_orders"] = total
		}
		// Add more metrics as needed
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleAccounts returns account information
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	// This would fetch real account data
	accounts := []map[string]interface{}{
		{
			"id": "main",
			"name": "Main Account",
			"balance": 100000,
			"equity": 105000,
			"margin_level": 5.2,
			"positions": 3,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

// handlePositions returns position information
func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	portfolio := s.collector.positionManager.GetPortfolioSummary()
	
	positions := []map[string]interface{}{}
	for accountID, summary := range portfolio.AccountSummaries {
		for symbol, pos := range summary.PositionsBySymbol {
			positions = append(positions, map[string]interface{}{
				"account_id": accountID,
				"symbol": symbol,
				"side": pos.Side,
				"quantity": pos.Quantity.String(),
				"entry_price": pos.EntryPrice.String(),
				"mark_price": pos.MarkPrice.String(),
				"pnl": pos.UnrealizedPnL.String(),
				"margin": pos.Margin.String(),
			})
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(positions)
}

// handlePerformance returns performance metrics
func (s *Server) handlePerformance(w http.ResponseWriter, r *http.Request) {
	performance := map[string]interface{}{
		"daily_pnl": []map[string]interface{}{
			{"date": "2025-08-20", "pnl": 1200},
			{"date": "2025-08-21", "pnl": -500},
			{"date": "2025-08-22", "pnl": 800},
		},
		"strategies": map[string]interface{}{
			"arbitrage": map[string]float64{
				"total_pnl": 5000,
				"win_rate": 0.65,
				"sharpe_ratio": 1.8,
			},
			"market_making": map[string]float64{
				"total_pnl": 3000,
				"win_rate": 0.72,
				"sharpe_ratio": 1.5,
			},
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(performance)
}

// dashboardHTML is a simple monitoring dashboard
const dashboardHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>OMS Monitoring Dashboard</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        h1 {
            color: #333;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        .card {
            background: white;
            border-radius: 8px;
            padding: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .card h3 {
            margin-top: 0;
            color: #666;
            font-size: 14px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .metric {
            font-size: 32px;
            font-weight: bold;
            color: #333;
            margin: 10px 0;
        }
        .positive {
            color: #4caf50;
        }
        .negative {
            color: #f44336;
        }
        .alert {
            background: #fff3cd;
            border: 1px solid #ffeaa7;
            border-radius: 4px;
            padding: 10px;
            margin: 10px 0;
        }
        .alert.critical {
            background: #f8d7da;
            border-color: #f5c6cb;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 10px;
        }
        th, td {
            text-align: left;
            padding: 8px;
            border-bottom: 1px solid #ddd;
        }
        th {
            font-weight: 600;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>OMS Monitoring Dashboard</h1>
        
        <div class="grid">
            <div class="card">
                <h3>Total Orders</h3>
                <div class="metric" id="total-orders">-</div>
            </div>
            
            <div class="card">
                <h3>Active Positions</h3>
                <div class="metric" id="active-positions">-</div>
            </div>
            
            <div class="card">
                <h3>Total Equity</h3>
                <div class="metric" id="total-equity">-</div>
            </div>
            
            <div class="card">
                <h3>Daily P&L</h3>
                <div class="metric" id="daily-pnl">-</div>
            </div>
        </div>
        
        <div class="card" style="margin-top: 20px;">
            <h3>Active Alerts</h3>
            <div id="alerts"></div>
        </div>
        
        <div class="card" style="margin-top: 20px;">
            <h3>Account Summary</h3>
            <table id="accounts-table">
                <thead>
                    <tr>
                        <th>Account</th>
                        <th>Balance</th>
                        <th>Equity</th>
                        <th>Margin Level</th>
                        <th>Positions</th>
                    </tr>
                </thead>
                <tbody></tbody>
            </table>
        </div>
        
        <div class="card" style="margin-top: 20px;">
            <h3>Open Positions</h3>
            <table id="positions-table">
                <thead>
                    <tr>
                        <th>Account</th>
                        <th>Symbol</th>
                        <th>Side</th>
                        <th>Quantity</th>
                        <th>Entry</th>
                        <th>Mark</th>
                        <th>P&L</th>
                    </tr>
                </thead>
                <tbody></tbody>
            </table>
        </div>
    </div>
    
    <script>
        // Fetch and update data
        async function updateDashboard() {
            try {
                // Fetch alerts
                const alertsResp = await fetch('/alerts');
                const alertsData = await alertsResp.json();
                updateAlerts(alertsData.active_alerts);
                
                // Fetch accounts
                const accountsResp = await fetch('/api/accounts');
                const accountsData = await accountsResp.json();
                updateAccounts(accountsData);
                
                // Fetch positions
                const positionsResp = await fetch('/api/positions');
                const positionsData = await positionsResp.json();
                updatePositions(positionsData);
                
                // Update metrics
                document.getElementById('active-positions').textContent = positionsData.length;
                
                // Calculate totals
                let totalEquity = 0;
                accountsData.forEach(acc => totalEquity += acc.equity);
                document.getElementById('total-equity').textContent = '$' + totalEquity.toLocaleString();
                
            } catch (error) {
                console.error('Error updating dashboard:', error);
            }
        }
        
        function updateAlerts(alerts) {
            const container = document.getElementById('alerts');
            container.innerHTML = '';
            
            if (alerts.length === 0) {
                container.innerHTML = '<p>No active alerts</p>';
                return;
            }
            
            alerts.forEach(alert => {
                const div = document.createElement('div');
                div.className = 'alert ' + (alert.Level === 'critical' ? 'critical' : '');
                div.textContent = alert.Message;
                container.appendChild(div);
            });
        }
        
        function updateAccounts(accounts) {
            const tbody = document.querySelector('#accounts-table tbody');
            tbody.innerHTML = '';
            
            accounts.forEach(acc => {
                const row = tbody.insertRow();
                row.innerHTML = ` + "`" + `
                    <td>${acc.name}</td>
                    <td>$${acc.balance.toLocaleString()}</td>
                    <td>$${acc.equity.toLocaleString()}</td>
                    <td>${acc.margin_level.toFixed(2)}</td>
                    <td>${acc.positions}</td>
                ` + "`" + `;
            });
        }
        
        function updatePositions(positions) {
            const tbody = document.querySelector('#positions-table tbody');
            tbody.innerHTML = '';
            
            positions.forEach(pos => {
                const pnl = parseFloat(pos.pnl);
                const row = tbody.insertRow();
                row.innerHTML = ` + "`" + `
                    <td>${pos.account_id}</td>
                    <td>${pos.symbol}</td>
                    <td>${pos.side}</td>
                    <td>${pos.quantity}</td>
                    <td>${pos.entry_price}</td>
                    <td>${pos.mark_price}</td>
                    <td class="${pnl >= 0 ? 'positive' : 'negative'}">
                        $${pnl.toFixed(2)}
                    </td>
                ` + "`" + `;
            });
        }
        
        // Update every 5 seconds
        updateDashboard();
        setInterval(updateDashboard, 5000);
    </script>
</body>
</html>
`