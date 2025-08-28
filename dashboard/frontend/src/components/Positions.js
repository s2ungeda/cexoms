import React, { useState, useEffect } from 'react';
import { Table, Card, Tag, Row, Col, Statistic, Progress, Button, Tabs } from 'antd';
import { ArrowUpOutlined, ArrowDownOutlined, DollarOutlined } from '@ant-design/icons';
import { Bar } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';

const { TabPane } = Tabs;

const Positions = () => {
  const [positions, setPositions] = useState([]);
  const [spotBalances, setSpotBalances] = useState([]);
  const [futuresPositions, setFuturesPositions] = useState([]);
  const [totalPnL, setTotalPnL] = useState(0);
  const [totalValue, setTotalValue] = useState(0);
  const [profitablePositions, setProfitablePositions] = useState(0);
  const [spotTotalValue, setSpotTotalValue] = useState(0);
  const [futuresUnrealizedPnL, setFuturesUnrealizedPnL] = useState(0);
  const [futuresAccountBalance, setFuturesAccountBalance] = useState(0);
  const [futuresAvailableBalance, setFuturesAvailableBalance] = useState(0);

  useEffect(() => {
    const handlePositionUpdate = (data) => {
      if (Array.isArray(data)) {
        setPositions(data);
        calculateMetrics(data);
      } else {
        // Single position update
        setPositions(prev => {
          const updated = [...prev];
          const index = updated.findIndex(p => p.symbol === data.symbol);
          if (index !== -1) {
            updated[index] = { ...updated[index], ...data };
          } else {
            updated.push(data);
          }
          calculateMetrics(updated);
          return updated;
        });
      }
    };

    const handleBalanceUpdate = (data) => {
      console.log('Balance update received:', data);
      // Parse data if it's a string
      const parsedData = typeof data === 'string' ? JSON.parse(data) : data;
      console.log('Parsed balance data:', parsedData);
      
      if (parsedData.balances) {
        console.log('Setting spot balances:', parsedData.balances);
        console.log('Total USD value:', parsedData.total_usd_value);
        setSpotBalances(parsedData.balances);
        setSpotTotalValue(parsedData.total_usd_value || 0);
      }
    };

    const handleFuturesPositionUpdate = (data) => {
      console.log('Futures position update received:', data);
      // Parse data if it's a string
      const parsedData = typeof data === 'string' ? JSON.parse(data) : data;
      console.log('Parsed futures data:', parsedData);
      
      if (parsedData.positions) {
        setFuturesPositions(parsedData.positions);
        setFuturesUnrealizedPnL(parsedData.total_unrealized_pnl || 0);
      }
      
      // Handle futures balances
      if (parsedData.account_type === 'futures') {
        console.log('Setting futures balances:', parsedData.account_balance, parsedData.available_balance);
        setFuturesAccountBalance(parsedData.account_balance || 0);
        setFuturesAvailableBalance(parsedData.available_balance || 0);
      }
    };

    WebSocketService.onPositionUpdate(handlePositionUpdate);
    WebSocketService.on('balance_update', handleBalanceUpdate);
    WebSocketService.on('futures_position_update', handleFuturesPositionUpdate);

    // Initial positions will come from real OMS via WebSocket

    return () => {
      WebSocketService.off('position_update', handlePositionUpdate);
      WebSocketService.off('balance_update', handleBalanceUpdate);
      WebSocketService.off('futures_position_update', handleFuturesPositionUpdate);
    };
  }, []);


  const calculateMetrics = (positionData) => {
    let totalUnrealizedPnL = 0;
    let totalPositionValue = 0;
    let profitCount = 0;

    positionData.forEach(pos => {
      totalUnrealizedPnL += pos.unrealizedPnL || 0;
      totalPositionValue += Math.abs(pos.value || 0);
      if ((pos.unrealizedPnL || 0) > 0) {
        profitCount++;
      }
    });

    setTotalPnL(totalUnrealizedPnL);
    setTotalValue(totalPositionValue);
    setProfitablePositions(profitCount);
  };

  const handleClosePosition = (symbol) => {
    console.log('Closing position:', symbol);
    // In real app, this would send a close order
  };

  const columns = [
    {
      title: 'Symbol',
      dataIndex: 'symbol',
      key: 'symbol',
      fixed: 'left',
      width: 100,
    },
    {
      title: 'Side',
      dataIndex: 'side',
      key: 'side',
      width: 80,
      render: (side) => (
        <Tag color={side === 'LONG' ? 'green' : 'red'}>{side}</Tag>
      ),
    },
    {
      title: 'Quantity',
      dataIndex: 'quantity',
      key: 'quantity',
      width: 100,
      align: 'right',
    },
    {
      title: 'Avg Price',
      dataIndex: 'avgPrice',
      key: 'avgPrice',
      width: 100,
      align: 'right',
      render: (price) => `$${price.toLocaleString()}`,
    },
    {
      title: 'Current Price',
      dataIndex: 'currentPrice',
      key: 'currentPrice',
      width: 120,
      align: 'right',
      render: (price, record) => {
        const isProfit = price > record.avgPrice;
        return (
          <span className={isProfit ? 'price-up' : 'price-down'}>
            ${price.toLocaleString()}
            {isProfit ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
          </span>
        );
      },
    },
    {
      title: 'Position Value',
      dataIndex: 'value',
      key: 'value',
      width: 120,
      align: 'right',
      render: (value) => `$${Math.abs(value).toLocaleString()}`,
    },
    {
      title: 'Unrealized PnL',
      dataIndex: 'unrealizedPnL',
      key: 'unrealizedPnL',
      width: 120,
      align: 'right',
      render: (pnl, record) => (
        <span className={pnl >= 0 ? 'positive' : 'negative'}>
          ${Math.abs(pnl).toFixed(2)} ({record.pnlPercent?.toFixed(2)}%)
        </span>
      ),
      sorter: (a, b) => a.unrealizedPnL - b.unrealizedPnL,
    },
    {
      title: 'Realized PnL',
      dataIndex: 'realizedPnL',
      key: 'realizedPnL',
      width: 120,
      align: 'right',
      render: (pnl) => (
        <span className={pnl >= 0 ? 'positive' : 'negative'}>
          ${Math.abs(pnl).toFixed(2)}
        </span>
      ),
    },
    {
      title: 'Exchange',
      dataIndex: 'exchange',
      key: 'exchange',
      width: 100,
    },
    {
      title: 'Action',
      key: 'action',
      fixed: 'right',
      width: 100,
      render: (record) => (
        <Button 
          size="small" 
          danger 
          onClick={() => handleClosePosition(record.symbol)}
        >
          Close
        </Button>
      ),
    },
  ];

  const pnlDistributionData = {
    labels: positions.map(p => p.symbol),
    datasets: [
      {
        label: 'Unrealized PnL',
        data: positions.map(p => p.unrealizedPnL),
        backgroundColor: positions.map(p => 
          p.unrealizedPnL >= 0 ? 'rgba(82, 196, 26, 0.8)' : 'rgba(245, 34, 45, 0.8)'
        ),
      },
    ],
  };

  const chartOptions = {
    responsive: true,
    plugins: {
      legend: {
        display: false,
      },
      title: {
        display: true,
        text: 'PnL Distribution by Position',
      },
    },
    scales: {
      y: {
        beginAtZero: true,
      },
    },
  };

  const rowClassName = (record) => {
    return record.unrealizedPnL >= 0 ? 'position-row-profit' : 'position-row-loss';
  };

  // Spot Balance columns
  const spotColumns = [
    {
      title: 'Asset',
      dataIndex: 'asset',
      key: 'asset',
      width: 100,
    },
    {
      title: 'Free',
      dataIndex: 'free',
      key: 'free',
      width: 120,
      align: 'right',
      render: (value) => value?.toFixed(8) || '0',
    },
    {
      title: 'Locked',
      dataIndex: 'locked',
      key: 'locked',
      width: 120,
      align: 'right',
      render: (value) => value?.toFixed(8) || '0',
    },
    {
      title: 'Total',
      dataIndex: 'total',
      key: 'total',
      width: 120,
      align: 'right',
      render: (value) => value?.toFixed(8) || '0',
    },
    {
      title: 'USD Value',
      dataIndex: 'usd_value',
      key: 'usd_value',
      width: 120,
      align: 'right',
      render: (value) => `$${value?.toFixed(2) || '0'}`,
    },
  ];

  // Futures Position columns
  const futuresColumns = [
    {
      title: 'Symbol',
      dataIndex: 'symbol',
      key: 'symbol',
      width: 100,
    },
    {
      title: 'Side',
      dataIndex: 'side',
      key: 'side',
      width: 80,
      render: (side) => (
        <Tag color={side === 'LONG' ? 'green' : 'red'}>{side}</Tag>
      ),
    },
    {
      title: 'Size',
      dataIndex: 'quantity',
      key: 'quantity',
      width: 100,
      align: 'right',
      render: (value) => value?.toFixed(3) || '0',
    },
    {
      title: 'Entry Price',
      dataIndex: 'entry_price',
      key: 'entry_price',
      width: 100,
      align: 'right',
      render: (price) => `$${price?.toFixed(2) || '0'}`,
    },
    {
      title: 'Mark Price',
      dataIndex: 'mark_price',
      key: 'mark_price',
      width: 100,
      align: 'right',
      render: (price) => `$${price?.toFixed(2) || '0'}`,
    },
    {
      title: 'PNL',
      dataIndex: 'unrealized_pnl',
      key: 'unrealized_pnl',
      width: 120,
      align: 'right',
      render: (pnl, record) => (
        <span className={pnl >= 0 ? 'positive' : 'negative'}>
          ${Math.abs(pnl || 0).toFixed(2)} ({(record.percentage || 0).toFixed(2)}%)
        </span>
      ),
    },
    {
      title: 'Margin',
      dataIndex: 'initial_margin',
      key: 'initial_margin',
      width: 100,
      align: 'right',
      render: (value) => `$${value?.toFixed(2) || '0'}`,
    },
    {
      title: 'Liq. Price',
      dataIndex: 'liquidation_price',
      key: 'liquidation_price',
      width: 100,
      align: 'right',
      render: (price) => price > 0 ? `$${price.toFixed(2)}` : '-',
    },
    {
      title: 'Leverage',
      dataIndex: 'leverage',
      key: 'leverage',
      width: 80,
      align: 'center',
      render: (value) => `${value}x`,
    },
  ];

  return (
    <div>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic
              title="Total Position Value"
              value={totalValue}
              prefix={<DollarOutlined />}
              precision={2}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Unrealized P&L"
              value={totalPnL}
              precision={2}
              valueStyle={{ color: totalPnL >= 0 ? '#3f8600' : '#cf1322' }}
              prefix={totalPnL >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
              suffix="USD"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Open Positions"
              value={positions.length}
              suffix={`(${profitablePositions} profitable)`}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Win Rate"
              value={(profitablePositions / positions.length * 100) || 0}
              suffix="%"
              precision={1}
            />
            <Progress 
              percent={(profitablePositions / positions.length * 100) || 0} 
              strokeColor="#52c41a" 
              showInfo={false}
            />
          </Card>
        </Col>
      </Row>

      <Card 
        style={{ marginTop: 16 }}
        title="Portfolio"
      >
        <Tabs defaultActiveKey="futures">
          <TabPane tab="Spot Balances" key="spot">
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={24}>
                <Card>
                  <Statistic
                    title="Total Spot Value"
                    value={spotTotalValue}
                    prefix={<DollarOutlined />}
                    precision={2}
                  />
                </Card>
              </Col>
            </Row>
            <Table
              columns={spotColumns}
              dataSource={spotBalances}
              rowKey="asset"
              pagination={false}
              scroll={{ x: 700 }}
            />
          </TabPane>
          
          <TabPane tab="Futures Positions" key="futures">
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={6}>
                <Card>
                  <Statistic
                    title="Account Balance"
                    value={futuresAccountBalance}
                    precision={2}
                    prefix={<DollarOutlined />}
                  />
                </Card>
              </Col>
              <Col span={6}>
                <Card>
                  <Statistic
                    title="Available Balance"
                    value={futuresAvailableBalance}
                    precision={2}
                    prefix={<DollarOutlined />}
                  />
                </Card>
              </Col>
              <Col span={6}>
                <Card>
                  <Statistic
                    title="Total Unrealized PNL"
                    value={futuresUnrealizedPnL}
                    precision={2}
                    valueStyle={{ color: futuresUnrealizedPnL >= 0 ? '#3f8600' : '#cf1322' }}
                    prefix={futuresUnrealizedPnL >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
                    suffix="USD"
                  />
                </Card>
              </Col>
              <Col span={6}>
                <Card>
                  <Statistic
                    title="Open Positions"
                    value={futuresPositions.length}
                    suffix="positions"
                  />
                </Card>
              </Col>
            </Row>
            <Table
              columns={futuresColumns}
              dataSource={futuresPositions}
              rowKey="symbol"
              rowClassName={(record) => record.unrealized_pnl >= 0 ? 'position-row-profit' : 'position-row-loss'}
              pagination={false}
              scroll={{ x: 1000 }}
            />
          </TabPane>
          
          <TabPane tab="Trading Positions" key="trading">
            <Table
              columns={columns}
              dataSource={positions}
              rowKey="symbol"
              rowClassName={rowClassName}
              scroll={{ x: 1300 }}
              pagination={false}
            />
          </TabPane>
        </Tabs>
      </Card>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="PnL Distribution">
            <Bar data={pnlDistributionData} options={chartOptions} />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Risk Exposure">
            <Row gutter={16}>
              <Col span={12}>
                <Statistic 
                  title="Long Exposure" 
                  value={positions.filter(p => p.side === 'LONG').reduce((sum, p) => sum + p.value, 0)}
                  prefix="$"
                  valueStyle={{ color: '#52c41a' }}
                />
              </Col>
              <Col span={12}>
                <Statistic 
                  title="Short Exposure" 
                  value={Math.abs(positions.filter(p => p.side === 'SHORT').reduce((sum, p) => sum + p.value, 0))}
                  prefix="$"
                  valueStyle={{ color: '#f5222d' }}
                />
              </Col>
            </Row>
            <div style={{ marginTop: 24 }}>
              <h4>Position Concentration</h4>
              {positions.map(pos => {
                const concentration = Math.abs(pos.value) / totalValue * 100;
                return (
                  <div key={pos.symbol} style={{ marginBottom: 8 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                      <span>{pos.symbol}</span>
                      <span>{concentration.toFixed(1)}%</span>
                    </div>
                    <Progress 
                      percent={concentration} 
                      showInfo={false}
                      strokeColor={concentration > 30 ? '#f5222d' : '#1890ff'}
                    />
                  </div>
                );
              })}
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default Positions;