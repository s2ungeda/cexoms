import React, { useState, useEffect } from 'react';
import { Row, Col, Card, Statistic, Table, Tag, Progress } from 'antd';
import { ArrowUpOutlined, ArrowDownOutlined, DollarOutlined } from '@ant-design/icons';
import { Line, Doughnut } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';
import moment from 'moment';

const Overview = () => {
  const [portfolioValue, setPortfolioValue] = useState(0);
  const [dailyPnL, setDailyPnL] = useState(0);
  const [openPositions, setOpenPositions] = useState(0);
  const [activeOrders, setActiveOrders] = useState(0);
  const [recentTrades, setRecentTrades] = useState([]);
  const [pnlHistory, setPnlHistory] = useState([]);
  const [positionDistribution, setPositionDistribution] = useState({});

  useEffect(() => {
    // WebSocket listeners
    const handleOrderUpdate = (data) => {
      // Update active orders count
      if (data.status === 'NEW') {
        setActiveOrders(prev => prev + 1);
      } else if (data.status === 'FILLED' || data.status === 'CANCELLED') {
        setActiveOrders(prev => Math.max(0, prev - 1));
        if (data.status === 'FILLED') {
          addRecentTrade(data);
        }
      }
    };

    const handlePositionUpdate = (data) => {
      // Update positions and PnL
      if (data.positions) {
        setOpenPositions(data.positions.length);
        updatePositionDistribution(data.positions);
        calculateTotalPnL(data.positions);
      }
    };

    WebSocketService.onOrderUpdate(handleOrderUpdate);
    WebSocketService.onPositionUpdate(handlePositionUpdate);

    // Initial data will come from real OMS via WebSocket

    return () => {
      WebSocketService.off('order_update', handleOrderUpdate);
      WebSocketService.off('position_update', handlePositionUpdate);
    };
  }, []);


  const addRecentTrade = (trade) => {
    setRecentTrades(prev => {
      const newTrade = {
        id: Date.now(),
        symbol: trade.symbol,
        side: trade.side,
        quantity: trade.quantity,
        price: trade.price,
        time: moment(),
        pnl: trade.pnl || 0,
      };
      return [newTrade, ...prev.slice(0, 9)];
    });
  };

  const updatePositionDistribution = (positions) => {
    const distribution = {};
    positions.forEach(pos => {
      distribution[pos.symbol] = (distribution[pos.symbol] || 0) + Math.abs(pos.value);
    });
    
    // Calculate percentages
    const total = Object.values(distribution).reduce((sum, val) => sum + val, 0);
    Object.keys(distribution).forEach(key => {
      distribution[key] = (distribution[key] / total * 100).toFixed(2);
    });
    
    setPositionDistribution(distribution);
  };

  const calculateTotalPnL = (positions) => {
    const totalPnL = positions.reduce((sum, pos) => sum + (pos.unrealizedPnL || 0), 0);
    setDailyPnL(totalPnL);
    setPortfolioValue(100000 + totalPnL);
  };

  // Chart configurations
  const pnlChartData = {
    labels: pnlHistory.map(item => item.time),
    datasets: [
      {
        label: 'Portfolio Value',
        data: pnlHistory.map(item => item.value),
        borderColor: 'rgb(75, 192, 192)',
        backgroundColor: 'rgba(75, 192, 192, 0.1)',
        tension: 0.4,
      },
    ],
  };

  const pnlChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      title: {
        display: true,
        text: '24h Portfolio Value',
      },
    },
    scales: {
      y: {
        beginAtZero: false,
      },
    },
  };

  const positionChartData = {
    labels: Object.keys(positionDistribution),
    datasets: [
      {
        data: Object.values(positionDistribution),
        backgroundColor: [
          'rgba(255, 99, 132, 0.8)',
          'rgba(54, 162, 235, 0.8)',
          'rgba(255, 206, 86, 0.8)',
          'rgba(75, 192, 192, 0.8)',
        ],
        borderWidth: 1,
      },
    ],
  };

  const positionChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'right',
      },
      title: {
        display: true,
        text: 'Position Distribution',
      },
    },
  };

  const tradeColumns = [
    {
      title: 'Time',
      dataIndex: 'time',
      key: 'time',
      render: (time) => moment(time).format('HH:mm:ss'),
    },
    {
      title: 'Symbol',
      dataIndex: 'symbol',
      key: 'symbol',
    },
    {
      title: 'Side',
      dataIndex: 'side',
      key: 'side',
      render: (side) => (
        <Tag color={side === 'BUY' ? 'green' : 'red'}>{side}</Tag>
      ),
    },
    {
      title: 'Quantity',
      dataIndex: 'quantity',
      key: 'quantity',
    },
    {
      title: 'Price',
      dataIndex: 'price',
      key: 'price',
      render: (price) => `$${price.toLocaleString()}`,
    },
    {
      title: 'PnL',
      dataIndex: 'pnl',
      key: 'pnl',
      render: (pnl) => (
        <span className={pnl >= 0 ? 'positive' : 'negative'}>
          ${Math.abs(pnl).toFixed(2)}
        </span>
      ),
    },
  ];

  return (
    <div>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic
              title="Portfolio Value"
              value={portfolioValue}
              precision={2}
              prefix={<DollarOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Daily P&L"
              value={dailyPnL}
              precision={2}
              valueStyle={{ color: dailyPnL >= 0 ? '#3f8600' : '#cf1322' }}
              prefix={dailyPnL >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
              suffix="USD"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Open Positions"
              value={openPositions}
              suffix="positions"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Active Orders"
              value={activeOrders}
              suffix="orders"
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 24 }}>
        <Col span={16}>
          <Card title="Portfolio Performance">
            <div style={{ height: 300 }}>
              <Line data={pnlChartData} options={pnlChartOptions} />
            </div>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="Position Distribution">
            <div style={{ height: 300 }}>
              <Doughnut data={positionChartData} options={positionChartOptions} />
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 24 }}>
        <Col span={24}>
          <Card title="Recent Trades">
            <Table
              dataSource={recentTrades}
              columns={tradeColumns}
              rowKey="id"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 24 }}>
        <Col span={24}>
          <Card title="Risk Metrics">
            <Row gutter={16}>
              <Col span={6}>
                <div className="metric-card">
                  <div className="metric-title">Max Drawdown</div>
                  <div className="metric-value">
                    -2.5<span className="metric-suffix">%</span>
                  </div>
                  <Progress percent={25} strokeColor="#f5222d" showInfo={false} />
                </div>
              </Col>
              <Col span={6}>
                <div className="metric-card">
                  <div className="metric-title">Win Rate</div>
                  <div className="metric-value">
                    65.4<span className="metric-suffix">%</span>
                  </div>
                  <Progress percent={65.4} strokeColor="#52c41a" showInfo={false} />
                </div>
              </Col>
              <Col span={6}>
                <div className="metric-card">
                  <div className="metric-title">Sharpe Ratio</div>
                  <div className="metric-value">1.82</div>
                </div>
              </Col>
              <Col span={6}>
                <div className="metric-card">
                  <div className="metric-title">Daily Volume</div>
                  <div className="metric-value">
                    $2.4<span className="metric-suffix">M</span>
                  </div>
                </div>
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default Overview;