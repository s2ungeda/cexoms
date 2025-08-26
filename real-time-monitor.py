#!/usr/bin/env python3

import asyncio
import json
import time
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
import threading
import nats
from collections import deque, defaultdict

# Global data storage
market_data = {
    'spot': defaultdict(lambda: {'ticker': {}, 'trades': deque(maxlen=100), 'orderbook': {}}),
    'futures': defaultdict(lambda: {'ticker': {}, 'trades': deque(maxlen=100), 'orderbook': {}})
}

stats = {
    'messages_received': 0,
    'last_update': None,
    'connected': False,
    'active_symbols': set()
}

class RealTimeMonitorHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/':
            self.send_response(200)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            
            html = f"""
<!DOCTYPE html>
<html>
<head>
    <title>OMS Real-Time Monitor</title>
    <meta charset="utf-8">
    <style>
        body {{
            font-family: monospace;
            background: #000;
            color: #0f0;
            padding: 20px;
            margin: 0;
        }}
        .container {{
            max-width: 1400px;
            margin: 0 auto;
        }}
        h1 {{
            color: #0ff;
            text-align: center;
            border-bottom: 2px solid #0ff;
            padding-bottom: 10px;
        }}
        .status {{
            background: #111;
            padding: 10px;
            border: 1px solid #0f0;
            margin-bottom: 20px;
        }}
        .grid {{
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
        }}
        .section {{
            background: #111;
            border: 1px solid #0f0;
            padding: 15px;
        }}
        .section h2 {{
            color: #ff0;
            margin-top: 0;
            border-bottom: 1px solid #ff0;
            padding-bottom: 5px;
        }}
        table {{
            width: 100%;
            border-collapse: collapse;
        }}
        th, td {{
            text-align: left;
            padding: 5px;
            border-bottom: 1px solid #333;
        }}
        th {{
            color: #0ff;
        }}
        .price-up {{
            color: #0f0;
        }}
        .price-down {{
            color: #f00;
        }}
        .timestamp {{
            color: #888;
            font-size: 0.9em;
        }}
        .symbol {{
            color: #ff0;
            font-weight: bold;
        }}
    </style>
    <script>
        function updateData() {{
            fetch('/data')
                .then(response => response.json())
                .then(data => {{
                    document.getElementById('status').innerHTML = data.status_html;
                    document.getElementById('spot-tickers').innerHTML = data.spot_tickers_html;
                    document.getElementById('futures-tickers').innerHTML = data.futures_tickers_html;
                    document.getElementById('recent-trades').innerHTML = data.recent_trades_html;
                }});
        }}
        setInterval(updateData, 1000);
        window.onload = updateData;
    </script>
</head>
<body>
    <div class="container">
        <h1>🔴 OMS Real-Time Market Monitor</h1>
        
        <div class="status" id="status">
            Connecting to NATS...
        </div>
        
        <div class="grid">
            <div class="section">
                <h2>📊 Spot Tickers</h2>
                <div id="spot-tickers">Loading...</div>
            </div>
            
            <div class="section">
                <h2>📈 Futures Tickers</h2>
                <div id="futures-tickers">Loading...</div>
            </div>
        </div>
        
        <div class="section" style="margin-top: 20px;">
            <h2>💹 Recent Trades (All Markets)</h2>
            <div id="recent-trades">Loading...</div>
        </div>
    </div>
</body>
</html>
"""
            self.wfile.write(html.encode())
            
        elif self.path == '/data':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            
            # Generate status HTML
            status_html = f"""
                <strong>NATS Connection:</strong> {'🟢 Connected' if stats['connected'] else '🔴 Disconnected'}<br>
                <strong>Messages Received:</strong> {stats['messages_received']:,}<br>
                <strong>Active Symbols:</strong> {len(stats['active_symbols'])}<br>
                <strong>Last Update:</strong> {stats['last_update'] or 'Never'}
            """
            
            # Generate spot tickers HTML
            spot_tickers_html = "<table><tr><th>Symbol</th><th>Price</th><th>24h Change</th><th>Volume</th></tr>"
            for symbol, data in sorted(market_data['spot'].items()):
                if data['ticker']:
                    ticker = data['ticker']
                    change_class = 'price-up' if float(ticker.get('priceChangePercent', 0)) > 0 else 'price-down'
                    spot_tickers_html += f"""
                        <tr>
                            <td class="symbol">{symbol}</td>
                            <td>${ticker.get('lastPrice', 'N/A')}</td>
                            <td class="{change_class}">{ticker.get('priceChangePercent', 'N/A')}%</td>
                            <td>{float(ticker.get('volume', 0)):,.0f}</td>
                        </tr>
                    """
            spot_tickers_html += "</table>"
            
            # Generate futures tickers HTML
            futures_tickers_html = "<table><tr><th>Symbol</th><th>Mark Price</th><th>24h Change</th><th>Volume</th></tr>"
            for symbol, data in sorted(market_data['futures'].items()):
                if data['ticker']:
                    ticker = data['ticker']
                    change_class = 'price-up' if float(ticker.get('priceChangePercent', 0)) > 0 else 'price-down'
                    futures_tickers_html += f"""
                        <tr>
                            <td class="symbol">{symbol}</td>
                            <td>${ticker.get('lastPrice', 'N/A')}</td>
                            <td class="{change_class}">{ticker.get('priceChangePercent', 'N/A')}%</td>
                            <td>{float(ticker.get('volume', 0)):,.0f}</td>
                        </tr>
                    """
            futures_tickers_html += "</table>"
            
            # Generate recent trades HTML
            all_trades = []
            for market in ['spot', 'futures']:
                for symbol, data in market_data[market].items():
                    for trade in data['trades']:
                        trade['market'] = market.upper()
                        trade['symbol'] = symbol
                        all_trades.append(trade)
            
            # Sort by timestamp and get last 20
            all_trades.sort(key=lambda x: x.get('time', 0), reverse=True)
            recent_trades = all_trades[:20]
            
            recent_trades_html = "<table><tr><th>Time</th><th>Market</th><th>Symbol</th><th>Side</th><th>Price</th><th>Quantity</th></tr>"
            for trade in recent_trades:
                side_color = 'price-up' if trade.get('isBuyerMaker', False) else 'price-down'
                timestamp = datetime.fromtimestamp(trade.get('time', 0) / 1000).strftime('%H:%M:%S')
                recent_trades_html += f"""
                    <tr>
                        <td class="timestamp">{timestamp}</td>
                        <td>{trade['market']}</td>
                        <td class="symbol">{trade['symbol']}</td>
                        <td class="{side_color}">{'BUY' if trade.get('isBuyerMaker', False) else 'SELL'}</td>
                        <td>${trade.get('price', 'N/A')}</td>
                        <td>{trade.get('qty', 'N/A')}</td>
                    </tr>
                """
            recent_trades_html += "</table>"
            
            response = {
                'status_html': status_html,
                'spot_tickers_html': spot_tickers_html,
                'futures_tickers_html': futures_tickers_html,
                'recent_trades_html': recent_trades_html
            }
            
            self.wfile.write(json.dumps(response).encode())

    def log_message(self, format, *args):
        pass  # Suppress logs

async def nats_subscriber():
    """Subscribe to NATS market data"""
    nc = await nats.connect("nats://localhost:4222")
    stats['connected'] = True
    print("Connected to NATS")
    
    async def message_handler(msg):
        stats['messages_received'] += 1
        stats['last_update'] = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        
        try:
            # Parse subject: market.data.{exchange}.{market}.{datatype}.{symbol}
            parts = msg.subject.split('.')
            if len(parts) >= 6:
                exchange = parts[2]
                market = parts[3]
                datatype = parts[4]
                symbol = parts[5]
                
                if exchange == 'binance' and market in ['spot', 'futures']:
                    data = json.loads(msg.data.decode())
                    stats['active_symbols'].add(symbol)
                    
                    if datatype == 'ticker':
                        market_data[market][symbol]['ticker'] = data
                    elif datatype == 'trade':
                        market_data[market][symbol]['trades'].append(data)
                    elif datatype == 'orderbook':
                        market_data[market][symbol]['orderbook'] = data
                        
        except Exception as e:
            print(f"Error processing message: {e}")
    
    # Subscribe to all market data
    await nc.subscribe("market.data.binance.>", cb=message_handler)
    
    print("Subscribed to market.data.binance.>")
    
    # Keep the connection alive
    while True:
        await asyncio.sleep(1)

def run_nats_subscriber():
    """Run NATS subscriber in asyncio loop"""
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    loop.run_until_complete(nats_subscriber())

def main():
    # Start NATS subscriber in a separate thread
    nats_thread = threading.Thread(target=run_nats_subscriber, daemon=True)
    nats_thread.start()
    
    # Start HTTP server
    server = HTTPServer(('localhost', 8081), RealTimeMonitorHandler)
    print("Real-time monitor running at http://localhost:8081")
    print("Subscribing to NATS market data...")
    server.serve_forever()

if __name__ == '__main__':
    main()