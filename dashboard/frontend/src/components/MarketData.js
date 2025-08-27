import React, { useState, useEffect, useRef } from 'react';
import { Card, Row, Col, Table, Statistic, Tag, Space, Select, Tabs } from 'antd';
import { ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import { Line } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';
import moment from 'moment';

const { Option } = Select;
const { TabPane } = Tabs;

const MarketData = () => {
  const [selectedSymbol, setSelectedSymbol] = useState('BTCUSDT');
  const [marketTickers, setMarketTickers] = useState({});
  const [orderBook, setOrderBook] = useState({ bids: [], asks: [] });
  const [priceHistory, setPriceHistory] = useState({});
  const [trades, setTrades] = useState([]);
  const chartRef = useRef(null);

  useEffect(() => {
    const handleMarketUpdate = (data) => {
      if (data.symbol) {
        // Update ticker
        setMarketTickers(prev => ({
          ...prev,
          [data.symbol]: {
            ...prev[data.symbol],
            ...data,
            lastUpdate: Date.now(),
          }
        }));

        // Update price history
        updatePriceHistory(data.symbol, data.price);
      }

      // Update order book if available
      if (data.orderBook) {
        setOrderBook(data.orderBook);
      }

      // Update recent trades
      if (data.trade) {
        addTrade(data.trade);
      }
    };

    WebSocketService.onMarketUpdate(handleMarketUpdate);

    // Initial market data will come from real OMS via WebSocket

    return () => {
      WebSocketService.off('market_update', handleMarketUpdate);
    };
  }, []);



  const updatePriceHistory = (symbol, price) => {
    setPriceHistory(prev => {
      const history = prev[symbol] || [];
      const newHistory = [...history];
      
      // Add new price point
      newHistory.push({
        time: moment().format('HH:mm:ss'),
        price,
      });

      // Keep only last 60 points
      if (newHistory.length > 60) {
        newHistory.shift();
      }

      return {
        ...prev,
        [symbol]: newHistory,
      };
    });
  };

  const addTrade = (trade) => {
    setTrades(prev => {
      const newTrades = [trade, ...prev];
      return newTrades.slice(0, 50); // Keep last 50 trades
    });
  };

  const getPriceChartData = () => {
    const history = priceHistory[selectedSymbol] || [];
    return {
      labels: history.map(h => h.time),
      datasets: [{
        label: selectedSymbol,
        data: history.map(h => h.price),
        borderColor: 'rgb(75, 192, 192)',
        backgroundColor: 'rgba(75, 192, 192, 0.1)',
        tension: 0.4,
      }],
    };
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      title: {
        display: true,
        text: `${selectedSymbol} Price Chart`,
      },
    },
    scales: {
      y: {
        beginAtZero: false,
      },
    },
  };

  const tickerColumns = [
    {
      title: 'Symbol',
      dataIndex: 'symbol',
      key: 'symbol',
      width: 100,
    },
    {
      title: 'Price',
      dataIndex: 'price',
      key: 'price',
      width: 100,
      render: (price, record) => (
        <span className={record.change24h >= 0 ? 'price-up' : 'price-down'}>
          ${price?.toFixed(2)}
          {record.change24h >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
        </span>
      ),
    },
    {
      title: '24h Change',
      dataIndex: 'change24h',
      key: 'change24h',
      width: 100,
      render: (change) => (
        <Tag color={change >= 0 ? 'green' : 'red'}>
          {change?.toFixed(2)}%
        </Tag>
      ),
    },
    {
      title: '24h Volume',
      dataIndex: 'volume24h',
      key: 'volume24h',
      width: 120,
      render: (vol) => `$${(vol / 1000).toFixed(1)}K`,
    },
  ];

  const tradesColumns = [
    {
      title: 'Time',
      dataIndex: 'time',
      key: 'time',
      width: 80,
      render: (time) => moment(time).format('HH:mm:ss'),
    },
    {
      title: 'Price',
      dataIndex: 'price',
      key: 'price',
      width: 80,
      render: (price) => `$${price.toFixed(2)}`,
    },
    {
      title: 'Quantity',
      dataIndex: 'quantity',
      key: 'quantity',
      width: 80,
      render: (qty) => qty.toFixed(4),
    },
    {
      title: 'Side',
      dataIndex: 'side',
      key: 'side',
      width: 60,
      render: (side) => (
        <Tag color={side === 'BUY' ? 'green' : 'red'} style={{ margin: 0 }}>
          {side}
        </Tag>
      ),
    },
  ];

  const currentTicker = marketTickers[selectedSymbol] || {};

  return (
    <div>
      <Row gutter={16}>
        <Col span={24}>
          <Card 
            title="Market Overview" 
            extra={
              <Select
                value={selectedSymbol}
                onChange={setSelectedSymbol}
                style={{ width: 120 }}
              >
                {Object.keys(marketTickers).map(symbol => (
                  <Option key={symbol} value={symbol}>{symbol}</Option>
                ))}
              </Select>
            }
          >
            <Row gutter={16}>
              <Col span={4}>
                <Statistic
                  title="Price"
                  value={currentTicker.price}
                  prefix="$"
                  precision={2}
                  valueStyle={{ color: currentTicker.change24h >= 0 ? '#3f8600' : '#cf1322' }}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h Change"
                  value={currentTicker.change24h}
                  suffix="%"
                  precision={2}
                  valueStyle={{ color: currentTicker.change24h >= 0 ? '#3f8600' : '#cf1322' }}
                  prefix={currentTicker.change24h >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h High"
                  value={currentTicker.high24h}
                  prefix="$"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h Low"
                  value={currentTicker.low24h}
                  prefix="$"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h Volume"
                  value={currentTicker.volume24h / 1000000}
                  suffix="M"
                  prefix="$"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="Spread"
                  value={currentTicker.ask - currentTicker.bid}
                  prefix="$"
                  precision={2}
                />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={16}>
          <Card title="Price Chart">
            <div style={{ height: 400 }}>
              <Line ref={chartRef} data={getPriceChartData()} options={chartOptions} />
            </div>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="Market Tickers" bodyStyle={{ padding: 0 }}>
            <Table
              dataSource={Object.values(marketTickers)}
              columns={tickerColumns}
              rowKey="symbol"
              pagination={false}
              size="small"
              rowClassName={(record) => 
                record.lastUpdate && Date.now() - record.lastUpdate < 1000 ? 'realtime-update' : ''
              }
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Order Book">
            <Tabs defaultActiveKey="1">
              <TabPane tab="Order Book" key="1">
                <div className="orderbook-container">
                  <div className="bids">
                    <div className="orderbook-row orderbook-header">
                      <span>Price</span>
                      <span>Quantity</span>
                      <span>Total</span>
                    </div>
                    {orderBook.bids.slice(0, 10).map((bid, index) => (
                      <div key={index} className="orderbook-row">
                        <span style={{ color: '#52c41a' }}>${bid.price.toFixed(2)}</span>
                        <span>{bid.quantity.toFixed(4)}</span>
                        <span>{bid.total.toFixed(4)}</span>
                      </div>
                    ))}
                  </div>
                  <div className="asks">
                    <div className="orderbook-row orderbook-header">
                      <span>Price</span>
                      <span>Quantity</span>
                      <span>Total</span>
                    </div>
                    {orderBook.asks.slice(0, 10).map((ask, index) => (
                      <div key={index} className="orderbook-row">
                        <span style={{ color: '#f5222d' }}>${ask.price.toFixed(2)}</span>
                        <span>{ask.quantity.toFixed(4)}</span>
                        <span>{ask.total.toFixed(4)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </TabPane>
            </Tabs>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Recent Trades" bodyStyle={{ padding: 0 }}>
            <Table
              dataSource={trades}
              columns={tradesColumns}
              rowKey="id"
              pagination={false}
              size="small"
              scroll={{ y: 400 }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default MarketData;