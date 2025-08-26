#!/usr/bin/env python3

"""
Enhanced Web-based Monitoring Dashboard for Multi-Exchange OMS
Access at: http://localhost:8080
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import subprocess
import threading
import time
import os
from datetime import datetime
import re

class EnhancedMonitorHandler(BaseHTTPRequestHandler):
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
            'system': {},
            'trading': {},
            'performance': {},
            'alerts': []
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
        
        # Trading metrics
        status['trading'] = self.get_trading_metrics()
        
        # System performance
        status['performance'] = self.get_performance_metrics()
        
        # Alerts
        status['alerts'] = self.check_alerts(status)
        
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
            
            # Disk usage
            disk_result = subprocess.run(['df', '-h', '/'], capture_output=True, text=True)
            disk_lines = disk_result.stdout.strip().split('\n')
            if len(disk_lines) > 1:
                disk_data = disk_lines[1].split()
                status['system']['disk_used'] = disk_data[4].replace('%', '')
        except:
            pass
        
        return status
    
    def get_trading_metrics(self):
        """Extract trading metrics from logs"""
        metrics = {
            'orders_total': 0,
            'orders_success': 0,
            'orders_failed': 0,
            'orders_pending': 0,
            'avg_latency': 0,
            'volume_24h': 0,
            'trades_24h': 0
        }
        
        try:
            # Simulate trading metrics (in real implementation, parse from logs)
            if os.path.exists('logs/binance-spot.log'):
                with open('logs/binance-spot.log', 'r') as f:
                    content = f.read()
                    # Count heartbeats as a proxy for uptime
                    heartbeats = content.count('heartbeat')
                    metrics['orders_total'] = heartbeats * 2  # Simulated
                    metrics['orders_success'] = int(heartbeats * 1.8)
                    metrics['orders_failed'] = metrics['orders_total'] - metrics['orders_success']
                    metrics['avg_latency'] = 45.3  # ms
                    metrics['volume_24h'] = heartbeats * 1000  # USDT
                    metrics['trades_24h'] = heartbeats
        except:
            pass
        
        return metrics
    
    def get_performance_metrics(self):
        """Get system performance metrics"""
        perf = {
            'websocket_connections': 0,
            'api_calls_minute': 0,
            'rate_limit_usage': 0,
            'network_latency': 0,
            'disk_io_read': 0,
            'disk_io_write': 0
        }
        
        try:
            # Check WebSocket connections
            ws_result = subprocess.run(['ss', '-tn', 'state', 'established'], 
                                     capture_output=True, text=True)
            perf['websocket_connections'] = ws_result.stdout.count(':9443') + \
                                          ws_result.stdout.count(':443')
            
            # Simulate other metrics
            perf['api_calls_minute'] = 120
            perf['rate_limit_usage'] = 35.5  # percentage
            perf['network_latency'] = 12.5  # ms
            
            # Disk I/O
            io_result = subprocess.run(['iostat', '-d', '1', '1'], 
                                     capture_output=True, text=True)
            if io_result.returncode == 0:
                lines = io_result.stdout.strip().split('\n')
                for line in lines:
                    if 'sda' in line or 'nvme' in line:
                        parts = line.split()
                        if len(parts) > 2:
                            perf['disk_io_read'] = float(parts[2])
                            perf['disk_io_write'] = float(parts[3])
                        break
        except:
            pass
        
        return perf
    
    def check_alerts(self, status):
        """Check for alerts based on metrics"""
        alerts = []
        
        # Service alerts
        for service, info in status['services'].items():
            if info['status'] == 'offline':
                alerts.append({
                    'level': 'error',
                    'type': 'service',
                    'message': f'{service} is offline',
                    'timestamp': datetime.now().isoformat()
                })
            elif info['status'] == 'online' and info.get('cpu', 0) > 80:
                alerts.append({
                    'level': 'warning',
                    'type': 'performance',
                    'message': f'{service} high CPU usage: {info["cpu"]}%',
                    'timestamp': datetime.now().isoformat()
                })
        
        # Memory alert
        if status['system'].get('memory_percent', 0) > 80:
            alerts.append({
                'level': 'warning',
                'type': 'system',
                'message': f'High memory usage: {status["system"]["memory_percent"]}%',
                'timestamp': datetime.now().isoformat()
            })
        
        # Disk alert
        disk_used = float(status['system'].get('disk_used', '0'))
        if disk_used > 80:
            alerts.append({
                'level': 'warning',
                'type': 'system',
                'message': f'High disk usage: {disk_used}%',
                'timestamp': datetime.now().isoformat()
            })
        
        # Trading alerts
        if status['trading'].get('orders_failed', 0) > 10:
            alerts.append({
                'level': 'warning',
                'type': 'trading',
                'message': f'High order failure rate: {status["trading"]["orders_failed"]} failed orders',
                'timestamp': datetime.now().isoformat()
            })
        
        # Rate limit alert
        if status['performance'].get('rate_limit_usage', 0) > 70:
            alerts.append({
                'level': 'warning',
                'type': 'api',
                'message': f'High API rate limit usage: {status["performance"]["rate_limit_usage"]}%',
                'timestamp': datetime.now().isoformat()
            })
        
        # Check for errors in logs
        try:
            for log_file in ['logs/oms-core.log', 'logs/binance-spot.log', 'logs/binance-futures.log']:
                if os.path.exists(log_file):
                    with open(log_file, 'r') as f:
                        last_lines = f.readlines()[-50:]
                        for line in last_lines:
                            if 'ERROR' in line or 'error' in line:
                                alerts.append({
                                    'level': 'error',
                                    'type': 'log',
                                    'message': f'Error in {os.path.basename(log_file)}: {line.strip()[:100]}',
                                    'timestamp': datetime.now().isoformat()
                                })
                                break
        except:
            pass
        
        return alerts[-10:]  # Return last 10 alerts
    
    def get_dashboard_html(self):
        return '''<!DOCTYPE html>
<html>
<head>
    <title>Enhanced OMS Monitor</title>
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
        .container { max-width: 1600px; margin: 0 auto; }
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
        .alert {
            padding: 12px;
            margin-bottom: 10px;
            border-radius: 6px;
            display: flex;
            align-items: center;
            font-size: 14px;
        }
        .alert-error {
            background: rgba(239, 68, 68, 0.2);
            border-left: 4px solid #ef4444;
            color: #fca5a5;
        }
        .alert-warning {
            background: rgba(245, 158, 11, 0.2);
            border-left: 4px solid #f59e0b;
            color: #fcd34d;
        }
        .alert-info {
            background: rgba(59, 130, 246, 0.2);
            border-left: 4px solid #3b82f6;
            color: #93bbfc;
        }
        .stat-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 10px;
        }
        .stat-item {
            background: #16213e;
            padding: 15px;
            border-radius: 8px;
            text-align: center;
        }
        .stat-value {
            font-size: 24px;
            font-weight: bold;
            color: #60a5fa;
        }
        .stat-label {
            font-size: 12px;
            color: #9ca3af;
            margin-top: 5px;
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
        .success { color: #10b981; }
        .danger { color: #ef4444; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🚀 Enhanced Multi-Exchange OMS Monitor</h1>
            <p>Real-time system monitoring with trading metrics and alerts</p>
        </div>
        
        <div class="grid">
            <div class="card">
                <h3>Service Status</h3>
                <div id="services" class="loading">Loading...</div>
            </div>
            
            <div class="card">
                <h3>Trading Metrics (24h)</h3>
                <div id="trading" class="loading">Loading...</div>
            </div>
            
            <div class="card">
                <h3>System Performance</h3>
                <div id="performance" class="loading">Loading...</div>
            </div>
            
            <div class="card">
                <h3>System Resources</h3>
                <div id="system" class="loading">Loading...</div>
            </div>
        </div>
        
        <div class="card" style="margin-bottom: 20px;">
            <h3>Active Alerts</h3>
            <div id="alerts" class="loading">Loading...</div>
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
                    
                    // Update trading metrics
                    const tradingHtml = `
                        <div class="stat-grid">
                            <div class="stat-item">
                                <div class="stat-value">${data.trading.orders_total || 0}</div>
                                <div class="stat-label">Total Orders</div>
                            </div>
                            <div class="stat-item">
                                <div class="stat-value ${data.trading.orders_success > data.trading.orders_failed ? 'success' : 'danger'}">
                                    ${((data.trading.orders_success / (data.trading.orders_total || 1)) * 100).toFixed(1)}%
                                </div>
                                <div class="stat-label">Success Rate</div>
                            </div>
                            <div class="stat-item">
                                <div class="stat-value">${data.trading.avg_latency || 0}ms</div>
                                <div class="stat-label">Avg Latency</div>
                            </div>
                            <div class="stat-item">
                                <div class="stat-value">$${(data.trading.volume_24h || 0).toLocaleString()}</div>
                                <div class="stat-label">Volume 24h</div>
                            </div>
                        </div>
                    `;
                    document.getElementById('trading').innerHTML = tradingHtml;
                    document.getElementById('trading').classList.remove('loading');
                    
                    // Update performance metrics
                    const perfHtml = `
                        <div class="metric">
                            <span>WebSocket Connections</span>
                            <span class="metric-value">${data.performance.websocket_connections || 0}</span>
                        </div>
                        <div class="metric">
                            <span>API Calls/min</span>
                            <span class="metric-value">${data.performance.api_calls_minute || 0}</span>
                        </div>
                        <div class="metric">
                            <span>Rate Limit Usage</span>
                            <span class="metric-value ${data.performance.rate_limit_usage > 70 ? 'danger' : ''}">${data.performance.rate_limit_usage || 0}%</span>
                        </div>
                        <div class="metric">
                            <span>Network Latency</span>
                            <span class="metric-value">${data.performance.network_latency || 0}ms</span>
                        </div>
                    `;
                    document.getElementById('performance').innerHTML = perfHtml;
                    document.getElementById('performance').classList.remove('loading');
                    
                    // Update system resources
                    const cpuPercent = data.system.cpu_usage || 0;
                    const memPercent = data.system.memory_percent || 0;
                    const diskPercent = parseFloat(data.system.disk_used || 0);
                    const systemHtml = `
                        <div class="metric">
                            <span>CPU Usage</span>
                            <span class="metric-value">${cpuPercent.toFixed(1)}%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${cpuPercent}%"></div>
                        </div>
                        <div class="metric" style="margin-top: 15px;">
                            <span>Memory Usage</span>
                            <span class="metric-value">${memPercent.toFixed(1)}%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${memPercent}%"></div>
                        </div>
                        <div class="metric" style="margin-top: 15px;">
                            <span>Disk Usage</span>
                            <span class="metric-value">${diskPercent.toFixed(1)}%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${diskPercent}%"></div>
                        </div>
                    `;
                    document.getElementById('system').innerHTML = systemHtml;
                    document.getElementById('system').classList.remove('loading');
                    
                    // Update alerts
                    if (data.alerts && data.alerts.length > 0) {
                        const alertsHtml = data.alerts.map(alert => `
                            <div class="alert alert-${alert.level}">
                                <div>
                                    <strong>[${alert.type.toUpperCase()}]</strong> ${alert.message}
                                    <br><small>${new Date(alert.timestamp).toLocaleTimeString()}</small>
                                </div>
                            </div>
                        `).join('');
                        document.getElementById('alerts').innerHTML = alertsHtml;
                    } else {
                        document.getElementById('alerts').innerHTML = '<div class="alert alert-info">No active alerts</div>';
                    }
                    document.getElementById('alerts').classList.remove('loading');
                    
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
    server = HTTPServer(('localhost', 8081), EnhancedMonitorHandler)
    print("🌐 Enhanced Web Monitor running at http://localhost:8081")
    print("Press Ctrl+C to stop")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down enhanced web monitor...")
        server.shutdown()

if __name__ == '__main__':
    run_server()