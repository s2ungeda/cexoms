import React, { useState, useEffect, useRef } from 'react';
import { Card, Row, Col, Table, Statistic, Tag, Space, Select, Tabs } from 'antd';
import { ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import { Line } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';
import SimpleCandlestickChart from './SimpleCandlestickChart';
import moment from 'moment';

const { Option } = Select;
const { TabPane } = Tabs;

const MarketData = () => {
  const [selectedSymbol, setSelectedSymbol] = useState('BTCUSDT');
  const [marketTickers, setMarketTickers] = useState({});
  const [priceHistory, setPriceHistory] = useState({});
  const chartRef = useRef(null);

  useEffect(() => {
    const handleMarketUpdate = (data) => {
      // Parse data if it's a string
      const parsedData = typeof data === 'string' ? JSON.parse(data) : data;
      
      if (parsedData.symbol) {
        // Ensure numeric values
        const tickerData = {
          ...parsedData,
          price: typeof parsedData.price === 'number' ? parsedData.price : parseFloat(parsedData.price),
          volume: typeof parsedData.volume === 'number' ? parsedData.volume : parseFloat(parsedData.volume),
          high: typeof parsedData.high === 'number' ? parsedData.high : parseFloat(parsedData.high),
          low: typeof parsedData.low === 'number' ? parsedData.low : parseFloat(parsedData.low),
          change: typeof parsedData.change === 'number' ? parsedData.change : parseFloat(parsedData.change),
          change_pct: typeof parsedData.change_pct === 'number' ? parsedData.change_pct : parseFloat(parsedData.change_pct),
          lastUpdate: Date.now(),
        };
        
        // Update ticker
        setMarketTickers(prev => ({
          ...prev,
          [parsedData.symbol]: tickerData
        }));

        // Update price history
        updatePriceHistory(parsedData.symbol, tickerData.price);
      }

      // Removed orderBook and trades handling to reduce system load
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
      dataIndex: 'change_pct',
      key: 'change_pct',
      width: 100,
      render: (change) => (
        <Tag color={(change || 0) >= 0 ? 'green' : 'red'}>
          {(change || 0).toFixed(2)}%
        </Tag>
      ),
    },
    {
      title: '24h Volume',
      dataIndex: 'volume',
      key: 'volume',
      width: 120,
      render: (vol) => `$${((vol || 0) / 1000000).toFixed(1)}M`,
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
                  value={currentTicker.change_pct || 0}
                  suffix="%"
                  precision={2}
                  valueStyle={{ color: (currentTicker.change_pct || 0) >= 0 ? '#3f8600' : '#cf1322' }}
                  prefix={(currentTicker.change_pct || 0) >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h High"
                  value={currentTicker.high || 0}
                  prefix="$"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h Low"
                  value={currentTicker.low || 0}
                  prefix="$"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="24h Volume"
                  value={(currentTicker.volume || 0) / 1000000}
                  suffix="M"
                  prefix="$"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="Spread"
                  value={(currentTicker.ask || currentTicker.price || 0) - (currentTicker.bid || currentTicker.price || 0)}
                  prefix="$"
                  precision={4}
                />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={24}>
          <SimpleCandlestickChart symbol={selectedSymbol} />
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={24}>
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
    </div>
  );
};

export default MarketData;