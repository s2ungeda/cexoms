package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type TickerData struct {
	Symbol             string `json:"s"`
	PriceChange        string `json:"p"`
	PriceChangePercent string `json:"P"`
	LastPrice          string `json:"c"`
	Volume             string `json:"v"`
	QuoteVolume        string `json:"q"`
	High               string `json:"h"`
	Low                string `json:"l"`
}

type MarketDisplay struct {
	mu      sync.RWMutex
	tickers map[string]TickerData
}

var display = &MarketDisplay{
	tickers: make(map[string]TickerData),
}

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>Live Binance Market Data</title>
    <style>
        body {
            font-family: monospace;
            background: #000;
            color: #0f0;
            margin: 0;
            padding: 20px;
        }
        h1 {
            text-align: center;
            color: #0ff;
            text-shadow: 0 0 10px #0ff;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th {
            background: #111;
            color: #ff0;
            padding: 10px;
            text-align: left;
            border: 1px solid #0f0;
        }
        td {
            padding: 8px;
            border: 1px solid #333;
        }
        tr:hover {
            background: #111;
        }
        .positive { color: #0f0; }
        .negative { color: #f00; }
        .symbol { color: #ff0; font-weight: bold; }
        .timestamp {
            text-align: center;
            color: #888;
            margin-top: 20px;
        }
        .live-indicator {
            display: inline-block;
            width: 10px;
            height: 10px;
            background: #f00;
            border-radius: 50%;
            animation: pulse 1s infinite;
            margin-right: 10px;
        }
        @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.3; }
            100% { opacity: 1; }
        }
    </style>
    <script>
        function refreshData() {
            fetch('/data')
                .then(response => response.json())
                .then(data => {
                    let html = '<table>';
                    html += '<tr><th>Symbol</th><th>Price</th><th>24h Change</th><th>24h %</th><th>24h Volume</th><th>High</th><th>Low</th></tr>';
                    
                    for (const [symbol, ticker] of Object.entries(data)) {
                        const changeClass = parseFloat(ticker.PriceChangePercent) >= 0 ? 'positive' : 'negative';
                        html += '<tr>';
                        html += '<td class="symbol">' + ticker.Symbol + '</td>';
                        html += '<td>$' + parseFloat(ticker.LastPrice).toFixed(4) + '</td>';
                        html += '<td class="' + changeClass + '">$' + parseFloat(ticker.PriceChange).toFixed(4) + '</td>';
                        html += '<td class="' + changeClass + '">' + parseFloat(ticker.PriceChangePercent).toFixed(2) + '%</td>';
                        html += '<td>' + parseInt(ticker.Volume).toLocaleString() + '</td>';
                        html += '<td>$' + parseFloat(ticker.High).toFixed(4) + '</td>';
                        html += '<td>$' + parseFloat(ticker.Low).toFixed(4) + '</td>';
                        html += '</tr>';
                    }
                    html += '</table>';
                    
                    document.getElementById('data').innerHTML = html;
                    document.getElementById('timestamp').textContent = new Date().toLocaleString();
                });
        }
        
        setInterval(refreshData, 1000);
        window.onload = refreshData;
    </script>
</head>
<body>
    <div class="container">
        <h1><span class="live-indicator"></span>Live Binance Market Data</h1>
        <div id="data">Loading...</div>
        <div class="timestamp">Last Update: <span id="timestamp"></span></div>
    </div>
</body>
</html>`

func connectWebSocket() {
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt", "adausdt", "dogeusdt", "maticusdt", "dotusdt", "shibusdt"}
	streams := []string{}
	for _, s := range symbols {
		streams = append(streams, s+"@ticker")
	}
	
	url := fmt.Sprintf("wss://stream.binance.com:9443/stream?streams=%s", strings.Join(streams, "/"))
	
	for {
		log.Println("Connecting to Binance WebSocket...")
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("dial error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		defer c.Close()
		
		log.Println("Connected to Binance WebSocket")
		
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Printf("read error: %v", err)
				break
			}
			
			var msg struct {
				Stream string          `json:"stream"`
				Data   json.RawMessage `json:"data"`
			}
			
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}
			
			var ticker TickerData
			if err := json.Unmarshal(msg.Data, &ticker); err != nil {
				continue
			}
			
			display.mu.Lock()
			display.tickers[ticker.Symbol] = ticker
			display.mu.Unlock()
		}
		
		log.Println("WebSocket disconnected, reconnecting...")
		time.Sleep(5 * time.Second)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlTemplate))
}

func handleData(w http.ResponseWriter, r *http.Request) {
	display.mu.RLock()
	data := make(map[string]TickerData)
	for k, v := range display.tickers {
		data[k] = v
	}
	display.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func main() {
	go connectWebSocket()
	
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/data", handleData)
	
	fmt.Println("Real-time Binance market data at http://localhost:8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}