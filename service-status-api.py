#!/usr/bin/env python3

import json
import subprocess
import os
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse

class ServiceStatusHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        # Enable CORS
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Content-type', 'application/json')
        self.end_headers()
        
        # Check services
        services = {
            'oms-core': {'name': 'OMS Core', 'process': 'oms-core'},
            'oms-server': {'name': 'OMS Server', 'process': 'oms-server'},
            'binance-spot': {'name': 'Binance Spot', 'process': 'binance-spot'},
            'binance-futures': {'name': 'Binance Futures', 'process': 'binance-futures'},
            'nats': {'name': 'NATS', 'process': 'nats-server'},
            'vault': {'name': 'Vault', 'process': 'vault'}
        }
        
        status = {}
        
        for service_id, service_info in services.items():
            try:
                # Check if process is running
                cmd = f"ps aux | grep '{service_info['process']}' | grep -v grep | head -1"
                result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
                
                if result.stdout:
                    fields = result.stdout.split()
                    pid = fields[1]
                    cpu = fields[2]
                    mem = fields[3]
                    
                    # Get uptime
                    uptime_cmd = f"ps -o etime= -p {pid}"
                    uptime_result = subprocess.run(uptime_cmd, shell=True, capture_output=True, text=True)
                    uptime = uptime_result.stdout.strip() if uptime_result.stdout else 'N/A'
                    
                    status[service_id] = {
                        'status': 'online',
                        'pid': pid,
                        'cpu': cpu + '%',
                        'memory': mem + '%',
                        'uptime': uptime
                    }
                else:
                    status[service_id] = {
                        'status': 'offline',
                        'pid': '-',
                        'cpu': '-',
                        'memory': '-',
                        'uptime': '-'
                    }
            except Exception as e:
                status[service_id] = {
                    'status': 'error',
                    'error': str(e)
                }
        
        # Get system resources
        try:
            # CPU usage
            cpu_cmd = "top -bn1 | grep 'Cpu(s)' | awk '{print $2}' | cut -d'%' -f1"
            cpu_result = subprocess.run(cpu_cmd, shell=True, capture_output=True, text=True)
            cpu_usage = cpu_result.stdout.strip() if cpu_result.stdout else 'N/A'
            
            # Memory usage
            mem_cmd = "free -m | grep Mem | awk '{print $3}'"
            mem_result = subprocess.run(mem_cmd, shell=True, capture_output=True, text=True)
            mem_usage = mem_result.stdout.strip() + ' MB' if mem_result.stdout else 'N/A'
            
            # Disk usage
            disk_cmd = "df -h / | tail -1 | awk '{print $5}'"
            disk_result = subprocess.run(disk_cmd, shell=True, capture_output=True, text=True)
            disk_usage = disk_result.stdout.strip() if disk_result.stdout else 'N/A'
            
            # WebSocket connections count
            # Count actual WebSocket connections from our services
            ws_count = 0
            
            # Check monitoring page WebSocket
            monitor_ws = subprocess.run("lsof -i TCP | grep -E 'stream.binance.com' | wc -l", 
                                       shell=True, capture_output=True, text=True)
            ws_count += int(monitor_ws.stdout.strip()) if monitor_ws.stdout.strip().isdigit() else 0
            
            # Check Binance Spot WebSocket
            spot_log = subprocess.run("tail -100 /home/seunge/project/mExOms/logs/binance-spot*.log 2>/dev/null | grep -i 'websocket connected' | wc -l", 
                                     shell=True, capture_output=True, text=True)
            if spot_log.stdout.strip() != '0':
                ws_count += 1
                
            # Check Binance Futures WebSocket
            futures_log = subprocess.run("grep -i 'Market data WebSocket connected' /home/seunge/project/mExOms/logs/binance-futures.log 2>/dev/null | wc -l", 
                                        shell=True, capture_output=True, text=True)
            if futures_log.stdout.strip() != '0':
                ws_count += 1
            
            ws_connections = str(ws_count)
            
            status['system'] = {
                'cpu': cpu_usage + '%',
                'memory': mem_usage,
                'disk': disk_usage,
                'websocket_connections': ws_connections
            }
        except:
            status['system'] = {
                'cpu': 'N/A',
                'memory': 'N/A',
                'disk': 'N/A',
                'websocket_connections': '0'
            }
        
        self.wfile.write(json.dumps(status).encode())
    
    def log_message(self, format, *args):
        pass  # Suppress logs

def main():
    server = HTTPServer(('localhost', 8083), ServiceStatusHandler)
    print("Service Status API running at http://localhost:8083")
    server.serve_forever()

if __name__ == '__main__':
    main()