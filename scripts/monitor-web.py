#!/usr/bin/env python3

"""
Web-based Monitoring Dashboard for Multi-Exchange OMS
Access at: http://localhost:8080
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import subprocess
import threading
import time
import os
from datetime import datetime

class MonitorHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/':
            self.send_response(200)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(self.get_dashboard_html().encode())
        elif self.path == '/api/status':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(json.dumps(self.get_system_status()).encode())
        else:
            self.send_error(404)
    
    def log_message(self, format, *args):
        pass  # Suppress request logs
    
    def get_system_status(self):
        status = {
            'timestamp': datetime.now().isoformat(),
            'services': {},
            'metrics': {},
            'system': {}
        }
        
        # Check services
        services = ['oms-core', 'oms-server', 'binance-spot', 'binance-futures']
        for service in services:
            try:
                result = subprocess.run(['pgrep', '-f', service], capture_output=True, text=True)
                if result.returncode == 0:
                    pid = result.stdout.strip().split('\n')[0]
                    ps_result = subprocess.run(['ps', '-p', pid, '-o', '%cpu,%mem,etime'], 
                                             capture_output=True, text=True)
                    if ps_result.returncode == 0:
                        stats = ps_result.stdout.strip().split('\n')[1].split()
                        status['services'][service] = {
                            'status': 'online',
                            'pid': pid,
                            'cpu': float(stats[0]),
                            'memory': float(stats[1]),
                            'uptime': stats[2]
                        }
                else:
                    status['services'][service] = {'status': 'offline'}
            except:
                status['services'][service] = {'status': 'error'}
        
        # Get metrics from logs
        try:
            if os.path.exists('logs/oms-core.log'):
                with open('logs/oms-core.log', 'r') as f:
                    lines = f.readlines()[-100:]  # Last 100 lines
                    for line in reversed(lines):
                        if 'Risk checks:' in line:
                            parts = line.split()
                            idx = parts.index('checks:')
                            status['metrics']['risk_checks'] = int(parts[idx + 1])
                            idx = parts.index('latency:')
                            status['metrics']['risk_latency'] = float(parts[idx + 1])
                            break
        except:
            pass
        
        # System resources
        try:
            # CPU
            cpu_result = subprocess.run(['top', '-bn1'], capture_output=True, text=True)
            for line in cpu_result.stdout.split('\n'):
                if 'Cpu(s)' in line:
                    status['system']['cpu_usage'] = float(line.split()[1].replace('%us,', ''))
                    break
            
            # Memory
            mem_result = subprocess.run(['free', '-m'], capture_output=True, text=True)
            mem_lines = mem_result.stdout.strip().split('\n')
            mem_data = mem_lines[1].split()
            status['system']['memory_total'] = int(mem_data[1])
            status['system']['memory_used'] = int(mem_data[2])
            status['system']['memory_percent'] = round(int(mem_data[2]) * 100 / int(mem_data[1]), 1)
        except:
            pass
        
        return status
    
    def get_dashboard_html(self):
        return '''<!DOCTYPE html>
<html>
<head>
    <title>Multi-Exchange OMS Monitor</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0f0f23;
            color: #e0e0e0;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 20px;
            border-radius: 10px;
            margin-bottom: 20px;
            box-shadow: 0 5px 20px rgba(102, 126, 234, 0.3);
        }
        h1 { font-size: 28px; font-weight: 600; }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .card {
            background: #1a1a2e;
            border-radius: 10px;
            padding: 20px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.3);
        }
        .card h3 {
            font-size: 16px;
            color: #9ca3af;
            margin-bottom: 15px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .service {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px;
            margin-bottom: 10px;
            background: #16213e;
            border-radius: 6px;
            transition: transform 0.2s;
        }
        .service:hover { transform: translateX(5px); }
        .service-name { font-weight: 500; }
        .status {
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 12px;
            font-weight: 600;
        }
        .online { background: #10b981; color: white; }
        .offline { background: #ef4444; color: white; }
        .metric {
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid #2a2a3e;
        }
        .metric:last-child { border-bottom: none; }
        .metric-value {
            font-weight: 600;
            color: #60a5fa;
        }
        .progress-bar {
            width: 100%;
            height: 20px;
            background: #2a2a3e;
            border-radius: 10px;
            overflow: hidden;
            margin-top: 10px;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #10b981 0%, #3b82f6 100%);
            transition: width 0.3s ease;
        }
        .timestamp {
            text-align: center;
            color: #6b7280;
            font-size: 14px;
            margin-top: 20px;
        }
        @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.5; }
            100% { opacity: 1; }
        }
        .loading { animation: pulse 2s infinite; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🚀 Multi-Exchange OMS Monitor</h1>
            <p>Real-time system monitoring dashboard</p>
        </div>
        
        <div class="grid">
            <div class="card">
                <h3>Service Status</h3>
                <div id="services" class="loading">Loading...</div>
            </div>
            
            <div class="card">
                <h3>Performance Metrics</h3>
                <div id="metrics" class="loading">Loading...</div>
            </div>
            
            <div class="card">
                <h3>System Resources</h3>
                <div id="system" class="loading">Loading...</div>
            </div>
        </div>
        
        <div class="timestamp" id="timestamp"></div>
    </div>
    
    <script>
        function updateDashboard() {
            fetch('/api/status')
                .then(response => response.json())
                .then(data => {
                    // Update services
                    const servicesHtml = Object.entries(data.services).map(([name, info]) => `
                        <div class="service">
                            <span class="service-name">${name}</span>
                            <span class="status ${info.status}">${info.status.toUpperCase()}</span>
                        </div>
                    `).join('');
                    document.getElementById('services').innerHTML = servicesHtml;
                    document.getElementById('services').classList.remove('loading');
                    
                    // Update metrics
                    const metricsHtml = `
                        <div class="metric">
                            <span>Risk Checks</span>
                            <span class="metric-value">${data.metrics.risk_checks || 0}</span>
                        </div>
                        <div class="metric">
                            <span>Risk Latency</span>
                            <span class="metric-value">${data.metrics.risk_latency || 0} μs</span>
                        </div>
                    `;
                    document.getElementById('metrics').innerHTML = metricsHtml;
                    document.getElementById('metrics').classList.remove('loading');
                    
                    // Update system resources
                    const cpuPercent = data.system.cpu_usage || 0;
                    const memPercent = data.system.memory_percent || 0;
                    const systemHtml = `
                        <div class="metric">
                            <span>CPU Usage</span>
                            <span class="metric-value">${cpuPercent.toFixed(1)}%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${cpuPercent}%"></div>
                        </div>
                        <div class="metric" style="margin-top: 20px;">
                            <span>Memory Usage</span>
                            <span class="metric-value">${memPercent.toFixed(1)}%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${memPercent}%"></div>
                        </div>
                    `;
                    document.getElementById('system').innerHTML = systemHtml;
                    document.getElementById('system').classList.remove('loading');
                    
                    // Update timestamp
                    document.getElementById('timestamp').textContent = 
                        'Last updated: ' + new Date(data.timestamp).toLocaleString();
                })
                .catch(error => console.error('Error fetching status:', error));
        }
        
        // Update every 2 seconds
        updateDashboard();
        setInterval(updateDashboard, 2000);
    </script>
</body>
</html>'''

def run_server():
    server = HTTPServer(('localhost', 8080), MonitorHandler)
    print("🌐 Web Monitor running at http://localhost:8080")
    print("Press Ctrl+C to stop")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down web monitor...")
        server.shutdown()

if __name__ == '__main__':
    run_server()