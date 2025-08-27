import ReconnectingWebSocket from 'reconnecting-websocket';

class WebSocketService {
  constructor() {
    this.ws = null;
    this.listeners = {};
    this.connectionListeners = {
      connect: [],
      disconnect: [],
    };
    this.messageQueue = [];
    this.connected = false;
  }

  connect(url) {
    this.ws = new ReconnectingWebSocket(url, [], {
      connectionTimeout: 5000,
      maxRetries: 10,
      maxReconnectionDelay: 10000,
      minReconnectionDelay: 1000,
      reconnectionDelayGrowFactor: 1.3,
    });

    this.ws.onopen = () => {
      console.log('WebSocket connected');
      this.connected = true;
      this.connectionListeners.connect.forEach(cb => cb());
      
      // Send queued messages
      this.messageQueue.forEach(msg => this.send(msg));
      this.messageQueue = [];
    };

    this.ws.onclose = () => {
      console.log('WebSocket disconnected');
      this.connected = false;
      this.connectionListeners.disconnect.forEach(cb => cb());
    };

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.handleMessage(message);
      } catch (error) {
        console.error('Failed to parse message:', error);
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
    }
  }

  send(message) {
    if (this.connected && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(message));
    } else {
      this.messageQueue.push(message);
    }
  }

  subscribe(streams) {
    this.send({
      type: 'subscribe',
      data: streams,
    });
  }

  unsubscribe(streams) {
    this.send({
      type: 'unsubscribe',
      data: streams,
    });
  }

  handleMessage(message) {
    const { type, data } = message;
    
    if (this.listeners[type]) {
      this.listeners[type].forEach(callback => {
        try {
          callback(data);
        } catch (error) {
          console.error(`Error in message handler for ${type}:`, error);
        }
      });
    }
  }

  on(type, callback) {
    if (!this.listeners[type]) {
      this.listeners[type] = [];
    }
    this.listeners[type].push(callback);
  }

  off(type, callback) {
    if (this.listeners[type]) {
      this.listeners[type] = this.listeners[type].filter(cb => cb !== callback);
    }
  }

  onConnect(callback) {
    this.connectionListeners.connect.push(callback);
  }

  onDisconnect(callback) {
    this.connectionListeners.disconnect.push(callback);
  }

  isConnected() {
    return this.connected;
  }

  // Helper methods for specific message types
  onOrderUpdate(callback) {
    this.on('order_update', callback);
  }

  onPositionUpdate(callback) {
    this.on('position_update', callback);
  }

  onMarketUpdate(callback) {
    this.on('market_update', callback);
  }

  onSystemMetrics(callback) {
    this.on('system_metrics', callback);
  }

  onRiskUpdate(callback) {
    this.on('risk_update', callback);
  }
}

export default new WebSocketService();