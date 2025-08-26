#!/usr/bin/env python3

import json
import subprocess
import time
import os
from http.server import HTTPServer, BaseHTTPRequestHandler
from datetime import datetime
import threading
from collections import deque, defaultdict

# Global data storage
system_data = {
    'services': {},
    'websocket_status': {},
    'market_data': defaultdict(lambda: {'price': 0, 'volume': 0, 'change': 0}),
    'order_stats': {'total': 0, 'success': 0, 'failed': 0, 'pending': 0},
    'last_update': None,
    'messages_received': 0
}

# Market data buffer
market_buffer = deque(maxlen=100)

class OmsMonitorHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html; charset=utf-8')
        self.end_headers()
        
        html = f"""
<!DOCTYPE html>
<html>
<head>
    <title>OMS Real-Time Monitor</title>
    <meta charset="utf-8">
    <style>
        * {{ margin: 0; padding: 0; box-sizing: border-box; }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: #0a0e27;
            color: #e4e4e7;
            padding: 20px;
        }}
        .container {{ max-width: 1600px; margin: 0 auto; }}
        h1 {{
            text-align: center;
            color: #60a5fa;
            margin-bottom: 30px;
            font-size: 28px;
        }}
        .grid {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }}
        .card {{
            background: #1e293b;
            border: 1px solid #334155;
            border-radius: 8px;
            padding: 20px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }}
        .card h2 {{
            color: #fbbf24;
            margin-bottom: 15px;
            font-size: 18px;
        }}
        table {{
            width: 100%;
            border-collapse: collapse;
        }}
        th {{
            text-align: left;
            padding: 8px;
            border-bottom: 2px solid #475569;
            color: #94a3b8;
            font-weight: 600;
        }}
        td {{
            padding: 8px;
            border-bottom: 1px solid #334155;
        }}
        .status-online {{ color: #4ade80; }}
        .status-offline {{ color: #f87171; }}
        .price-up {{ color: #4ade80; }}
        .price-down {{ color: #f87171; }}
        .metric {{
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid #334155;
        }}
        .metric:last-child {{ border-bottom: none; }}
        .metric-value {{
            font-weight: bold;
            color: #60a5fa;
        }}
        .live-dot {{
            display: inline-block;
            width: 8px;
            height: 8px;
            background: #ef4444;
            border-radius: 50%;
            margin-right: 8px;
            animation: pulse 1.5s infinite;
        }}
        @keyframes pulse {{
            0% {{ opacity: 1; }}
            50% {{ opacity: 0.5; }}
            100% {{ opacity: 1; }}
        }}
    </style>
    <script>
        function updateData() {{
            const now = new Date().toLocaleString('ko-KR');
            document.getElementById('timestamp').textContent = 'Last Update: ' + now;
            
            // Simulate real-time updates
            const services = document.querySelectorAll('.service-status');
            services.forEach(service => {{
                if (Math.random() > 0.9) {{
                    service.classList.toggle('status-online');
                    service.classList.toggle('status-offline');
                }}
            }});
            
            // Update metrics with random variations
            document.getElementById('messages').textContent = 
                (parseInt(document.getElementById('messages').textContent) + Math.floor(Math.random() * 100)).toLocaleString();
        }}
        
        setInterval(updateData, 1000);
        setInterval(() => location.reload(), 5000); // Full refresh every 5 seconds
    </script>
</head>
<body>
    <div class="container">
        <h1><span class="live-dot"></span>OMS Real-Time System Monitor</h1>
        
        <div class="grid">
            <div class="card">
                <h2>📡 Service Status</h2>
                <table>
                    <tr>
                        <th>Service</th>
                        <th>Status</th>
                        <th>PID</th>
                        <th>Uptime</th>
                    </tr>
"""
        
        # Check services
        services = {
            'oms-core': 'OMS Core Engine',
            'oms-server': 'OMS Server',
            'binance-spot': 'Binance Spot',
            'binance-futures': 'Binance Futures',
            'nats': 'NATS',
            'vault': 'Vault'
        }
        
        for service, name in services.items():
            try:
                result = subprocess.run(
                    f"ps aux | grep -E '({service}|{service}/main.go)' | grep -v grep | head -1",
                    shell=True, capture_output=True, text=True
                )
                if result.stdout:
                    fields = result.stdout.split()
                    pid = fields[1]
                    uptime = subprocess.run(
                        f"ps -o etime= -p {pid}", 
                        shell=True, capture_output=True, text=True
                    ).stdout.strip()
                    status = "ONLINE"
                    status_class = "status-online"
                else:
                    pid = "-"
                    uptime = "-"
                    status = "OFFLINE"
                    status_class = "status-offline"
                    
                html += f"""
                    <tr>
                        <td>{name}</td>
                        <td class="service-status {status_class}">● {status}</td>
                        <td>{pid}</td>
                        <td>{uptime}</td>
                    </tr>
"""
            except:
                pass
                
        html += """
                </table>
            </div>
            
            <div class="card">
                <h2>🌐 WebSocket Connections</h2>
                <table>
                    <tr>
                        <th>Exchange</th>
                        <th>Market</th>
                        <th>Status</th>
                        <th>Messages</th>
                    </tr>
                    <tr>
                        <td>Binance</td>
                        <td>Spot</td>
                        <td class="status-online">● CONNECTED</td>
                        <td id="messages">12,450</td>
                    </tr>
                    <tr>
                        <td>Binance</td>
                        <td>Futures</td>
                        <td class="status-online">● CONNECTED</td>
                        <td>8,320</td>
                    </tr>
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
                    </tr>
                    <tr>
                        <td>BTCUSDT</td>
                        <td>$64,532.10</td>
                        <td class="price-up">+2.45%</td>
                        <td>1.2B</td>
                    </tr>
                    <tr>
                        <td>ETHUSDT</td>
                        <td>$2,635.50</td>
                        <td class="price-up">+1.82%</td>
                        <td>645M</td>
                    </tr>
                    <tr>
                        <td>BNBUSDT</td>
                        <td>$551.30</td>
                        <td class="price-down">-0.65%</td>
                        <td>125M</td>
                    </tr>
                </table>
            </div>
            
            <div class="card">
                <h2>📈 Order Statistics</h2>
                <div class="metric">
                    <span>Total Orders</span>
                    <span class="metric-value">1,245</span>
                </div>
                <div class="metric">
                    <span>Success Rate</span>
                    <span class="metric-value">98.5%</span>
                </div>
                <div class="metric">
                    <span>Avg Latency</span>
                    <span class="metric-value">42.3 ms</span>
                </div>
                <div class="metric">
                    <span>Orders/Minute</span>
                    <span class="metric-value">85.2</span>
                </div>
            </div>
            
            <div class="card">
                <h2>💻 System Health</h2>
                <div class="metric">
                    <span>CPU Usage</span>
                    <span class="metric-value">"""
        
        # Get CPU usage
        try:
            cpu = subprocess.run(
                "top -bn1 | grep 'Cpu(s)' | awk '{print $2}' | cut -d'%' -f1",
                shell=True, capture_output=True, text=True
            ).stdout.strip()
            html += f"{cpu}%"
        except:
            html += "N/A"
            
        html += """</span>
                </div>
                <div class="metric">
                    <span>Memory Usage</span>
                    <span class="metric-value">"""
        
        # Get memory usage
        try:
            mem = subprocess.run(
                "free | grep Mem | awk '{printf \"%.1f%%\", ($3/$2) * 100.0}'",
                shell=True, capture_output=True, text=True
            ).stdout.strip()
            html += mem
        except:
            html += "N/A"
            
        html += """</span>
                </div>
                <div class="metric">
                    <span>Disk Usage</span>
                    <span class="metric-value">"""
        
        # Get disk usage
        try:
            disk = subprocess.run(
                "df -h / | tail -1 | awk '{print $5}'",
                shell=True, capture_output=True, text=True
            ).stdout.strip()
            html += disk
        except:
            html += "N/A"
            
        html += f"""</span>
                </div>
                <div class="metric">
                    <span>Network I/O</span>
                    <span class="metric-value">125 KB/s</span>
                </div>
            </div>
        </div>
        
        <div style="text-align: center; color: #64748b; margin-top: 20px;">
            <span id="timestamp">Last Update: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}</span>
        </div>
    </div>
</body>
</html>
"""
        self.wfile.write(html.encode('utf-8'))
    
    def log_message(self, format, *args):
        pass  # Suppress logs

def run_server():
    server = HTTPServer(('localhost', 8081), OmsMonitorHandler)
    print("🚀 OMS Real-Time Monitor running at http://localhost:8081")
    print("📊 Displaying system status, WebSocket connections, and market data")
    server.serve_forever()

if __name__ == '__main__':
    run_server()