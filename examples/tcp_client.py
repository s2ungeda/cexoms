#!/usr/bin/env python3
"""
mExOms TCP Client Example in Python
"""

import socket
import struct
import threading
import time
import sys
from typing import Optional, Callable
import json

class TcpClient:
    """Simple TCP client for mExOms server"""
    
    def __init__(self, host: str = 'localhost', port: int = 9090):
        self.host = host
        self.port = port
        self.socket: Optional[socket.socket] = None
        self.connected = False
        self.receive_thread: Optional[threading.Thread] = None
        self.running = False
        self.message_callback: Optional[Callable] = None
        
        # Statistics
        self.messages_sent = 0
        self.messages_received = 0
        
    def connect(self) -> bool:
        """Connect to TCP server"""
        try:
            self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self.socket.connect((self.host, self.port))
            self.connected = True
            self.running = True
            
            # Start receive thread
            self.receive_thread = threading.Thread(target=self._receive_loop)
            self.receive_thread.daemon = True
            self.receive_thread.start()
            
            print(f"Connected to {self.host}:{self.port}")
            return True
            
        except Exception as e:
            print(f"Connection failed: {e}")
            return False
            
    def disconnect(self):
        """Disconnect from server"""
        self.running = False
        self.connected = False
        
        if self.socket:
            try:
                self.socket.shutdown(socket.SHUT_RDWR)
                self.socket.close()
            except:
                pass
                
        if self.receive_thread and self.receive_thread.is_alive():
            self.receive_thread.join(timeout=1)
            
        print("Disconnected")
        
    def send_message(self, message: bytes) -> bool:
        """Send message with length header"""
        if not self.connected or not self.socket:
            return False
            
        try:
            # Add length header (4 bytes, big endian)
            length = len(message)
            header = struct.pack('>I', length)
            
            # Send header + message
            self.socket.sendall(header + message)
            self.messages_sent += 1
            return True
            
        except Exception as e:
            print(f"Send error: {e}")
            return False
            
    def send_json(self, data: dict) -> bool:
        """Send JSON message (for testing)"""
        json_str = json.dumps(data)
        return self.send_message(json_str.encode())
        
    def _receive_loop(self):
        """Receive messages from server"""
        buffer = bytearray()
        
        while self.running and self.connected:
            try:
                # Receive data
                data = self.socket.recv(4096)
                if not data:
                    print("Server disconnected")
                    break
                    
                buffer.extend(data)
                
                # Process complete messages
                while len(buffer) >= 4:
                    # Extract message length
                    msg_len = struct.unpack('>I', buffer[:4])[0]
                    
                    # Check if complete message available
                    if len(buffer) < 4 + msg_len:
                        break
                        
                    # Extract message
                    message = buffer[4:4+msg_len]
                    buffer = buffer[4+msg_len:]
                    
                    self.messages_received += 1
                    
                    # Process message
                    if self.message_callback:
                        self.message_callback(message)
                    else:
                        print(f"Received: {message}")
                        
            except socket.timeout:
                continue
            except Exception as e:
                print(f"Receive error: {e}")
                break
                
        self.connected = False
        
    def set_message_callback(self, callback: Callable):
        """Set callback for received messages"""
        self.message_callback = callback
        
    def get_stats(self) -> dict:
        """Get connection statistics"""
        return {
            'connected': self.connected,
            'messages_sent': self.messages_sent,
            'messages_received': self.messages_received
        }


# Example usage
def main():
    """Test the TCP client"""
    
    # Parse command line arguments
    host = sys.argv[1] if len(sys.argv) > 1 else 'localhost'
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 9090
    
    # Create client
    client = TcpClient(host, port)
    
    # Message handler
    def on_message(message: bytes):
        try:
            # Try to decode as JSON
            data = json.loads(message.decode())
            print(f"Received JSON: {data}")
        except:
            # Raw message
            print(f"Received: {message}")
    
    client.set_message_callback(on_message)
    
    # Connect
    if not client.connect():
        return
        
    try:
        print("\n=== mExOms TCP Client Test ===")
        
        # Test messages (using JSON for simplicity)
        test_messages = [
            {
                'type': 'login',
                'api_key': 'test_key_123',
                'secret': 'test_secret'
            },
            {
                'type': 'subscribe',
                'symbols': ['BTC/USDT', 'ETH/USDT'],
                'channels': ['ticker', 'trades']
            },
            {
                'type': 'order',
                'symbol': 'BTC/USDT',
                'side': 'buy',
                'quantity': 0.001,
                'price': 65000
            }
        ]
        
        # Send test messages
        for i, msg in enumerate(test_messages):
            print(f"\nSending message {i+1}...")
            if client.send_json(msg):
                print(f"Sent: {msg}")
                time.sleep(1)
                
        # Wait for responses
        print("\nWaiting for responses...")
        time.sleep(5)
        
        # Show statistics
        stats = client.get_stats()
        print(f"\n=== Statistics ===")
        print(f"Messages sent: {stats['messages_sent']}")
        print(f"Messages received: {stats['messages_received']}")
        
        # Interactive mode
        print("\n=== Interactive Mode ===")
        print("Type messages to send (JSON format) or 'quit' to exit")
        
        while True:
            try:
                user_input = input("\n> ")
                
                if user_input.lower() == 'quit':
                    break
                    
                # Try to parse as JSON
                try:
                    msg = json.loads(user_input)
                    if client.send_json(msg):
                        print("Sent!")
                except json.JSONDecodeError:
                    # Send as plain text
                    if client.send_message(user_input.encode()):
                        print("Sent as plain text!")
                        
            except KeyboardInterrupt:
                break
                
    finally:
        # Disconnect
        client.disconnect()
        
        # Final stats
        stats = client.get_stats()
        print(f"\nFinal statistics:")
        print(f"  Total sent: {stats['messages_sent']}")
        print(f"  Total received: {stats['messages_received']}")


if __name__ == '__main__':
    main()