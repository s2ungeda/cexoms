/**
 * OMS Client - JavaScript/Node.js SDK for Multi-Exchange OMS
 */

const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');

// Load proto file
const PROTO_PATH = path.join(__dirname, '../../proto/oms.proto');
const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});

const omsProto = grpc.loadPackageDefinition(packageDefinition).oms;

class OMSClient {
  /**
   * Initialize OMS client
   * @param {string} serverAddress - gRPC server address (default: localhost:50051)
   */
  constructor(serverAddress = 'localhost:50051') {
    this.client = new omsProto.OrderService(
      serverAddress,
      grpc.credentials.createInsecure()
    );
  }

  /**
   * Place a new order
   * @param {Object} params - Order parameters
   * @returns {Promise<Object>} Order response
   */
  placeOrder(params) {
    return new Promise((resolve, reject) => {
      const request = {
        symbol: params.symbol,
        side: params.side,
        order_type: params.orderType || 'LIMIT',
        quantity: params.quantity,
        price: params.price || 0,
        exchange: params.exchange || 'binance',
        market: params.market || 'spot',
        account_id: params.accountId || 'main'
      };

      this.client.PlaceOrder(request, (err, response) => {
        if (err) {
          reject(err);
        } else {
          resolve({
            orderId: response.order_id,
            exchangeOrderId: response.exchange_order_id,
            status: response.status,
            createdAt: new Date(parseInt(response.created_at) * 1000)
          });
        }
      });
    });
  }

  /**
   * Cancel an existing order
   * @param {string} orderId - Order ID to cancel
   * @returns {Promise<Object>} Cancel response
   */
  cancelOrder(orderId) {
    return new Promise((resolve, reject) => {
      const request = { order_id: orderId };

      this.client.CancelOrder(request, (err, response) => {
        if (err) {
          reject(err);
        } else {
          resolve({
            orderId: response.order_id,
            status: response.status,
            cancelledAt: new Date(parseInt(response.cancelled_at) * 1000)
          });
        }
      });
    });
  }

  /**
   * Get order details
   * @param {string} orderId - Order ID
   * @returns {Promise<Object>} Order details
   */
  getOrder(orderId) {
    return new Promise((resolve, reject) => {
      const request = { order_id: orderId };

      this.client.GetOrder(request, (err, response) => {
        if (err) {
          reject(err);
        } else {
          resolve(this._formatOrder(response.order));
        }
      });
    });
  }

  /**
   * List orders with optional filters
   * @param {Object} filters - Filter options
   * @returns {Promise<Array>} List of orders
   */
  listOrders(filters = {}) {
    return new Promise((resolve, reject) => {
      const request = {
        status: filters.status || '',
        symbol: filters.symbol || ''
      };

      this.client.ListOrders(request, (err, response) => {
        if (err) {
          reject(err);
        } else {
          resolve(response.orders.map(order => this._formatOrder(order)));
        }
      });
    });
  }

  /**
   * Get account balance
   * @param {Object} params - Balance parameters
   * @returns {Promise<Object>} Balance object
   */
  getBalance(params = {}) {
    return new Promise((resolve, reject) => {
      const request = {
        exchange: params.exchange || 'binance',
        market: params.market || 'spot',
        account_id: params.accountId || 'main'
      };

      this.client.GetBalance(request, (err, response) => {
        if (err) {
          reject(err);
        } else {
          const balances = {};
          response.balances.forEach(balance => {
            balances[balance.asset] = {
              free: parseFloat(balance.free),
              locked: parseFloat(balance.locked),
              total: parseFloat(balance.free) + parseFloat(balance.locked)
            };
          });
          resolve(balances);
        }
      });
    });
  }

  /**
   * Get open positions (futures)
   * @param {Object} params - Position parameters
   * @returns {Promise<Array>} List of positions
   */
  getPositions(params = {}) {
    return new Promise((resolve, reject) => {
      const request = {
        exchange: params.exchange || 'binance',
        account_id: params.accountId || 'main'
      };

      this.client.GetPositions(request, (err, response) => {
        if (err) {
          reject(err);
        } else {
          const positions = response.positions.map(pos => ({
            symbol: pos.symbol,
            side: pos.side,
            size: parseFloat(pos.size),
            entryPrice: parseFloat(pos.entry_price),
            markPrice: parseFloat(pos.mark_price),
            unrealizedPnl: parseFloat(pos.unrealized_pnl),
            pnlPercentage: parseFloat(pos.pnl_percentage),
            leverage: parseInt(pos.leverage),
            margin: parseFloat(pos.margin)
          }));
          resolve(positions);
        }
      });
    });
  }

  /**
   * Stream real-time prices
   * @param {Array<string>} symbols - Symbols to stream
   * @param {Function} callback - Callback for price updates
   * @returns {Object} Stream object
   */
  streamPrices(symbols, callback) {
    const request = { symbols };
    const stream = this.client.StreamPrices(request);

    stream.on('data', (response) => {
      callback({
        exchange: response.exchange,
        symbol: response.symbol,
        bidPrice: parseFloat(response.bid_price),
        bidQuantity: parseFloat(response.bid_quantity),
        askPrice: parseFloat(response.ask_price),
        askQuantity: parseFloat(response.ask_quantity),
        lastPrice: parseFloat(response.last_price),
        timestamp: new Date(parseInt(response.timestamp) * 1000)
      });
    });

    stream.on('error', (err) => {
      console.error('Stream error:', err);
    });

    stream.on('end', () => {
      console.log('Stream ended');
    });

    return stream;
  }

  /**
   * Stream order updates
   * @param {Function} callback - Callback for order updates
   * @returns {Object} Stream object
   */
  streamOrders(callback) {
    const request = {};
    const stream = this.client.StreamOrders(request);

    stream.on('data', (response) => {
      callback(this._formatOrder(response.order));
    });

    stream.on('error', (err) => {
      console.error('Stream error:', err);
    });

    stream.on('end', () => {
      console.log('Stream ended');
    });

    return stream;
  }

  /**
   * Format order object
   * @private
   */
  _formatOrder(order) {
    return {
      orderId: order.order_id,
      exchangeOrderId: order.exchange_order_id,
      symbol: order.symbol,
      side: order.side,
      orderType: order.order_type,
      quantity: parseFloat(order.quantity),
      price: parseFloat(order.price),
      filledQuantity: parseFloat(order.filled_quantity),
      status: order.status,
      exchange: order.exchange,
      market: order.market,
      accountId: order.account_id,
      createdAt: new Date(parseInt(order.created_at) * 1000),
      updatedAt: order.updated_at ? new Date(parseInt(order.updated_at) * 1000) : null
    };
  }
}

// Example usage
async function example() {
  const client = new OMSClient('localhost:50051');

  try {
    // Place a limit order
    console.log('Placing order...');
    const orderResult = await client.placeOrder({
      symbol: 'BTCUSDT',
      side: 'BUY',
      quantity: 0.001,
      price: 115000,
      orderType: 'LIMIT'
    });
    console.log('Order placed:', orderResult);

    // Get order details
    const order = await client.getOrder(orderResult.orderId);
    console.log('Order details:', order);

    // Get balance
    const balance = await client.getBalance();
    console.log('Balance:', balance);

    // Stream prices
    console.log('Streaming prices...');
    const priceStream = client.streamPrices(['BTCUSDT', 'ETHUSDT'], (update) => {
      console.log(`Price update: ${update.symbol} - Bid: $${update.bidPrice.toFixed(2)}, Ask: $${update.askPrice.toFixed(2)}`);
    });

    // Stop streaming after 10 seconds
    setTimeout(() => {
      priceStream.cancel();
      console.log('Stopped streaming');
    }, 10000);

  } catch (error) {
    console.error('Error:', error);
  }
}

// Export client
module.exports = OMSClient;

// Run example if called directly
if (require.main === module) {
  example().catch(console.error);
}