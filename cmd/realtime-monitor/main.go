package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type MarketData struct {
	mu      sync.RWMutex
	Tickers map[string]map[string]interface{} // market -> symbol -> ticker data
	Trades  map[string][]Trade                // market -> trades
	Stats   Stats
}

type Trade struct {
	Symbol        string    `json:"symbol"`
	Price         string    `json:"price"`
	Quantity      string    `json:"qty"`
	Time          int64     `json:"time"`
	IsBuyerMaker  bool      `json:"isBuyerMaker"`
	Timestamp     time.Time `json:"timestamp"`
}

type Stats struct {
	Connected        bool
	MessagesReceived int64
	LastUpdate       time.Time
	ActiveSymbols    map[string]bool
}

var marketData = &MarketData{
	Tickers:       make(map[string]map[string]interface{}),
	Trades:        make(map[string][]Trade),
	Stats:         Stats{ActiveSymbols: make(map[string]bool)},
}

const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
    <title>OMS Real-Time Monitor</title>
    <style>
        body {
            font-family: 'Courier New', monospace;
            background: #0a0a0a;
            color: #00ff00;
            padding: 20px;
            margin: 0;
        }
        .container {
            max-width: 1600px;
            margin: 0 auto;
        }
        h1 {
            color: #00ffff;
            text-align: center;
            text-shadow: 0 0 10px #00ffff;
            margin-bottom: 30px;
        }
        .status {
            background: #1a1a1a;
            padding: 15px;
            border: 2px solid #00ff00;
            margin-bottom: 20px;
            border-radius: 5px;
            box-shadow: 0 0 10px rgba(0,255,0,0.3);
        }
        .grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
            margin-bottom: 20px;
        }
        .section {
            background: #1a1a1a;
            border: 1px solid #00ff00;
            padding: 15px;
            border-radius: 5px;
            box-shadow: 0 0 10px rgba(0,255,0,0.2);
        }
        h2 {
            color: #ffff00;
            margin-top: 0;
            border-bottom: 1px solid #ffff00;
            padding-bottom: 10px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th {
            color: #00ffff;
            text-align: left;
            padding: 8px;
            border-bottom: 2px solid #00ffff;
        }
        td {
            padding: 6px 8px;
            border-bottom: 1px solid #333;
        }
        .price-up { color: #00ff00; }
        .price-down { color: #ff0000; }
        .symbol { color: #ffff00; font-weight: bold; }
        .timestamp { color: #888888; font-size: 0.9em; }
        .blink {
            animation: blink 1s linear infinite;
        }
        @keyframes blink {
            0% { opacity: 1; }
            50% { opacity: 0; }
            100% { opacity: 1; }
        }
    </style>
    <script>
        function updateData() {
            fetch('/api/data')
                .then(response => response.json())
                .then(data => {
                    // Update status
                    document.getElementById('status').innerHTML = 
                        '<strong>NATS:</strong> ' + (data.connected ? '🟢 Connected' : '🔴 Disconnected') + ' | ' +
                        '<strong>Messages:</strong> ' + data.messages.toLocaleString() + ' | ' +
                        '<strong>Active Symbols:</strong> ' + data.activeSymbols + ' | ' +
                        '<strong>Last Update:</strong> ' + (data.lastUpdate || 'Never');
                    
                    // Update spot tickers
                    let spotHTML = '<table><tr><th>Symbol</th><th>Price</th><th>24h Change</th><th>Volume</th></tr>';
                    for (const [symbol, ticker] of Object.entries(data.spotTickers || {})) {
                        const changeClass = parseFloat(ticker.priceChangePercent || 0) > 0 ? 'price-up' : 'price-down';
                        spotHTML += '<tr>' +
                            '<td class="symbol">' + symbol + '</td>' +
                            '<td>$' + (ticker.lastPrice || 'N/A') + '</td>' +
                            '<td class="' + changeClass + '">' + (ticker.priceChangePercent || 'N/A') + '%</td>' +
                            '<td>' + parseFloat(ticker.volume || 0).toLocaleString(undefined, {maximumFractionDigits: 0}) + '</td>' +
                            '</tr>';
                    }
                    spotHTML += '</table>';
                    document.getElementById('spot-tickers').innerHTML = spotHTML;
                    
                    // Update futures tickers
                    let futuresHTML = '<table><tr><th>Symbol</th><th>Mark Price</th><th>24h Change</th><th>Volume</th></tr>';
                    for (const [symbol, ticker] of Object.entries(data.futuresTickers || {})) {
                        const changeClass = parseFloat(ticker.priceChangePercent || 0) > 0 ? 'price-up' : 'price-down';
                        futuresHTML += '<tr>' +
                            '<td class="symbol">' + symbol + '</td>' +
                            '<td>$' + (ticker.lastPrice || 'N/A') + '</td>' +
                            '<td class="' + changeClass + '">' + (ticker.priceChangePercent || 'N/A') + '%</td>' +
                            '<td>' + parseFloat(ticker.volume || 0).toLocaleString(undefined, {maximumFractionDigits: 0}) + '</td>' +
                            '</tr>';
                    }
                    futuresHTML += '</table>';
                    document.getElementById('futures-tickers').innerHTML = futuresHTML;
                    
                    // Update recent trades
                    let tradesHTML = '<table><tr><th>Time</th><th>Market</th><th>Symbol</th><th>Side</th><th>Price</th><th>Quantity</th></tr>';
                    for (const trade of data.recentTrades || []) {
                        const sideClass = trade.isBuyerMaker ? 'price-down' : 'price-up';
                        const side = trade.isBuyerMaker ? 'SELL' : 'BUY';
                        tradesHTML += '<tr>' +
                            '<td class="timestamp">' + trade.timestamp + '</td>' +
                            '<td>' + trade.market + '</td>' +
                            '<td class="symbol">' + trade.symbol + '</td>' +
                            '<td class="' + sideClass + '">' + side + '</td>' +
                            '<td>$' + trade.price + '</td>' +
                            '<td>' + trade.quantity + '</td>' +
                            '</tr>';
                    }
                    tradesHTML += '</table>';
                    document.getElementById('recent-trades').innerHTML = tradesHTML;
                });
        }
        
        setInterval(updateData, 500); // Update every 500ms for real-time feel
        window.onload = updateData;
    </script>
</head>
<body>
    <div class="container">
        <h1>🔴 <span class="blink">LIVE</span> OMS Real-Time Market Monitor</h1>
        
        <div class="status" id="status">
            Connecting to NATS...
        </div>
        
        <div class="grid">
            <div class="section">
                <h2>📊 Binance Spot</h2>
                <div id="spot-tickers">Loading...</div>
            </div>
            
            <div class="section">
                <h2>📈 Binance Futures</h2>
                <div id="futures-tickers">Loading...</div>
            </div>
        </div>
        
        <div class="section">
            <h2>💹 Live Trade Feed</h2>
            <div id="recent-trades">Loading...</div>
        </div>
    </div>
</body>
</html>`

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	tmpl.Execute(w, nil)
}

func handleAPIData(w http.ResponseWriter, r *http.Request) {
	marketData.mu.RLock()
	defer marketData.mu.RUnlock()

	// Get tickers
	spotTickers := make(map[string]interface{})
	futuresTickers := make(map[string]interface{})
	
	for market, symbols := range marketData.Tickers {
		for symbol, ticker := range symbols {
			if market == "spot" {
				spotTickers[symbol] = ticker
			} else if market == "futures" {
				futuresTickers[symbol] = ticker
			}
		}
	}

	// Get recent trades (last 20 from all markets)
	allTrades := []map[string]interface{}{}
	for market, trades := range marketData.Trades {
		marketUpper := "SPOT"
		if market == "futures" {
			marketUpper = "FUTURES"
		}
		
		start := 0
		if len(trades) > 10 {
			start = len(trades) - 10
		}
		
		for i := start; i < len(trades); i++ {
			trade := trades[i]
			allTrades = append(allTrades, map[string]interface{}{
				"market":       marketUpper,
				"symbol":       trade.Symbol,
				"price":        trade.Price,
				"quantity":     trade.Quantity,
				"isBuyerMaker": trade.IsBuyerMaker,
				"timestamp":    trade.Timestamp.Format("15:04:05"),
			})
		}
	}

	// Sort trades by time (newest first)
	if len(allTrades) > 20 {
		allTrades = allTrades[len(allTrades)-20:]
	}

	// Reverse array to show newest first
	for i, j := 0, len(allTrades)-1; i < j; i, j = i+1, j-1 {
		allTrades[i], allTrades[j] = allTrades[j], allTrades[i]
	}

	response := map[string]interface{}{
		"connected":      marketData.Stats.Connected,
		"messages":       marketData.Stats.MessagesReceived,
		"activeSymbols":  len(marketData.Stats.ActiveSymbols),
		"lastUpdate":     marketData.Stats.LastUpdate.Format("2006-01-02 15:04:05"),
		"spotTickers":    spotTickers,
		"futuresTickers": futuresTickers,
		"recentTrades":   allTrades,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func subscribeToNATS() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Printf("Failed to connect to NATS: %v", err)
		return
	}
	defer nc.Close()

	marketData.mu.Lock()
	marketData.Stats.Connected = true
	marketData.mu.Unlock()

	log.Println("Connected to NATS")

	// Subscribe to all market data
	nc.Subscribe("market.data.binance.>", func(msg *nats.Msg) {
		marketData.mu.Lock()
		marketData.Stats.MessagesReceived++
		marketData.Stats.LastUpdate = time.Now()
		marketData.mu.Unlock()

		// Parse subject: market.data.{exchange}.{market}.{datatype}.{symbol}
		parts := []string{}
		subject := msg.Subject
		for i := 0; i < len(subject); i++ {
			start := i
			for i < len(subject) && subject[i] != '.' {
				i++
			}
			parts = append(parts, subject[start:i])
		}

		if len(parts) >= 6 && parts[2] == "binance" {
			market := parts[3]
			dataType := parts[4]
			symbol := parts[5]

			marketData.mu.Lock()
			marketData.Stats.ActiveSymbols[symbol] = true

			// Initialize maps if needed
			if marketData.Tickers[market] == nil {
				marketData.Tickers[market] = make(map[string]interface{})
			}

			switch dataType {
			case "ticker":
				var ticker map[string]interface{}
				if err := json.Unmarshal(msg.Data, &ticker); err == nil {
					marketData.Tickers[market][symbol] = ticker
				}

			case "trade":
				var trade Trade
				if err := json.Unmarshal(msg.Data, &trade); err == nil {
					trade.Symbol = symbol
					trade.Timestamp = time.Now()
					
					// Keep only last 100 trades per market
					if len(marketData.Trades[market]) >= 100 {
						marketData.Trades[market] = marketData.Trades[market][1:]
					}
					marketData.Trades[market] = append(marketData.Trades[market], trade)
				}
			}
			marketData.mu.Unlock()
		}
	})

	// Keep the connection alive
	select {}
}

func main() {
	// Start NATS subscriber in a goroutine
	go subscribeToNATS()

	// HTTP handlers
	http.HandleFunc("/", handleDashboard)
	http.HandleFunc("/api/data", handleAPIData)

	fmt.Println("Real-time monitor running at http://localhost:8081")
	fmt.Println("Subscribing to NATS market data streams...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}