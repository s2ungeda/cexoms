import React, { useState, useEffect } from 'react';
import { Table, Card, Tag, Row, Col, Statistic, Progress, Button } from 'antd';
import { ArrowUpOutlined, ArrowDownOutlined, DollarOutlined } from '@ant-design/icons';
import { Bar } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';

const Positions = () => {
  const [positions, setPositions] = useState([]);
  const [totalPnL, setTotalPnL] = useState(0);
  const [totalValue, setTotalValue] = useState(0);
  const [profitablePositions, setProfitablePositions] = useState(0);

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

    WebSocketService.onPositionUpdate(handlePositionUpdate);

    // Initial positions will come from real OMS via WebSocket

    return () => {
      WebSocketService.off('position_update', handlePositionUpdate);
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

  const handleCloseAll = () => {
    console.log('Closing all positions');
    // In real app, this would send close orders for all positions
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
        title="Open Positions"
        extra={
          <Button danger onClick={handleCloseAll}>
            Close All Positions
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={positions}
          rowKey="symbol"
          rowClassName={rowClassName}
          scroll={{ x: 1300 }}
          pagination={false}
        />
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