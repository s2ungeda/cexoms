import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Progress, Statistic, Table, Tag, Alert, Space, Button } from 'antd';
import { WarningOutlined, SafetyOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import { Line, Radar } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';
import moment from 'moment';

const RiskManagement = () => {
  const [riskMetrics, setRiskMetrics] = useState({
    portfolioVaR: 0,
    portfolioCVaR: 0,
    maxDrawdown: 0,
    currentDrawdown: 0,
    sharpeRatio: 0,
    totalExposure: 0,
    leverage: 0,
    marginUsage: 0,
  });

  const [exposureLimits, setExposureLimits] = useState([]);

  const [alerts, setAlerts] = useState([]);

  const [drawdownHistory, setDrawdownHistory] = useState([]);
  const [varHistory, setVarHistory] = useState([]);

  useEffect(() => {
    const handleRiskUpdate = (data) => {
      if (data.metrics) {
        setRiskMetrics(prev => ({ ...prev, ...data.metrics }));
      }

      if (data.exposures) {
        updateExposureLimits(data.exposures);
      }

      if (data.alert) {
        addAlert(data.alert);
      }
    };

    WebSocketService.onRiskUpdate(handleRiskUpdate);

    // Initial data will come from real OMS via WebSocket

    return () => {
      WebSocketService.off('risk_update', handleRiskUpdate);
    };
  }, []);


  const updateExposureLimits = (exposures) => {
    setExposureLimits(exposures);
  };

  const addAlert = (alert) => {
    setAlerts(prev => [
      { ...alert, id: Date.now(), time: moment() },
      ...prev.slice(0, 9)
    ]);
  };

  const getRiskLevel = (metric, value) => {
    if (metric === 'drawdown') {
      if (value > 10) return { level: 'high', color: '#f5222d' };
      if (value > 5) return { level: 'medium', color: '#faad14' };
      return { level: 'low', color: '#52c41a' };
    }
    if (metric === 'leverage') {
      if (value > 3) return { level: 'high', color: '#f5222d' };
      if (value > 2) return { level: 'medium', color: '#faad14' };
      return { level: 'low', color: '#52c41a' };
    }
    if (metric === 'margin') {
      if (value > 80) return { level: 'high', color: '#f5222d' };
      if (value > 60) return { level: 'medium', color: '#faad14' };
      return { level: 'low', color: '#52c41a' };
    }
    return { level: 'normal', color: '#1890ff' };
  };

  // Chart configurations
  const drawdownChartData = {
    labels: drawdownHistory.map(d => d.time),
    datasets: [{
      label: 'Drawdown %',
      data: drawdownHistory.map(d => d.value),
      borderColor: 'rgb(255, 99, 132)',
      backgroundColor: 'rgba(255, 99, 132, 0.1)',
      tension: 0.4,
    }],
  };

  const varChartData = {
    labels: varHistory.map(d => d.time),
    datasets: [
      {
        label: 'VaR (95%)',
        data: varHistory.map(d => d.var95),
        borderColor: 'rgb(255, 206, 86)',
        backgroundColor: 'rgba(255, 206, 86, 0.1)',
        tension: 0.4,
      },
      {
        label: 'CVaR (95%)',
        data: varHistory.map(d => d.cvar95),
        borderColor: 'rgb(255, 99, 132)',
        backgroundColor: 'rgba(255, 99, 132, 0.1)',
        tension: 0.4,
      },
    ],
  };

  const riskRadarData = {
    labels: ['Drawdown', 'Leverage', 'Margin', 'VaR', 'Concentration', 'Volatility'],
    datasets: [{
      label: 'Current Risk Profile',
      data: [
        (riskMetrics.currentDrawdown / riskMetrics.maxDrawdown) * 100,
        (riskMetrics.leverage / 3) * 100,
        riskMetrics.marginUsage,
        (riskMetrics.portfolioVaR / 5000) * 100,
        75, // Concentration
        60, // Volatility
      ],
      backgroundColor: 'rgba(255, 99, 132, 0.2)',
      borderColor: 'rgba(255, 99, 132, 1)',
      pointBackgroundColor: 'rgba(255, 99, 132, 1)',
    }],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'bottom',
      },
    },
  };

  const exposureColumns = [
    {
      title: 'Asset',
      dataIndex: 'asset',
      key: 'asset',
    },
    {
      title: 'Current Exposure',
      dataIndex: 'current',
      key: 'current',
      render: (val) => `$${val.toLocaleString()}`,
    },
    {
      title: 'Limit',
      dataIndex: 'limit',
      key: 'limit',
      render: (val) => `$${val.toLocaleString()}`,
    },
    {
      title: 'Usage',
      key: 'usage',
      render: (record) => {
        const risk = getRiskLevel('exposure', record.usage);
        return (
          <div>
            <Progress 
              percent={record.usage} 
              strokeColor={risk.color}
              format={percent => `${percent.toFixed(1)}%`}
            />
          </div>
        );
      },
    },
    {
      title: 'Status',
      key: 'status',
      render: (record) => {
        if (record.usage > 90) return <Tag color="error">Critical</Tag>;
        if (record.usage > 75) return <Tag color="warning">Warning</Tag>;
        return <Tag color="success">Normal</Tag>;
      },
    },
  ];

  const alertColumns = [
    {
      title: 'Time',
      dataIndex: 'time',
      key: 'time',
      width: 120,
      render: (time) => moment(time).format('HH:mm:ss'),
    },
    {
      title: 'Alert',
      dataIndex: 'message',
      key: 'message',
      render: (msg, record) => (
        <Space>
          {record.type === 'warning' && <WarningOutlined style={{ color: '#faad14' }} />}
          {record.type === 'error' && <ExclamationCircleOutlined style={{ color: '#f5222d' }} />}
          {record.type === 'info' && <SafetyOutlined style={{ color: '#1890ff' }} />}
          {msg}
        </Space>
      ),
    },
  ];

  const drawdownRisk = getRiskLevel('drawdown', riskMetrics.currentDrawdown);
  const leverageRisk = getRiskLevel('leverage', riskMetrics.leverage);
  const marginRisk = getRiskLevel('margin', riskMetrics.marginUsage);

  return (
    <div>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic
              title="Portfolio VaR (95%)"
              value={riskMetrics.portfolioVaR}
              prefix="$"
              precision={0}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Current Drawdown"
              value={riskMetrics.currentDrawdown}
              suffix="%"
              precision={1}
              valueStyle={{ color: drawdownRisk.color }}
            />
            <Progress 
              percent={riskMetrics.currentDrawdown} 
              strokeColor={drawdownRisk.color}
              showInfo={false}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Leverage"
              value={riskMetrics.leverage}
              suffix="x"
              precision={2}
              valueStyle={{ color: leverageRisk.color }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Margin Usage"
              value={riskMetrics.marginUsage}
              suffix="%"
              precision={1}
              valueStyle={{ color: marginRisk.color }}
            />
            <Progress 
              percent={riskMetrics.marginUsage} 
              strokeColor={marginRisk.color}
              showInfo={false}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Drawdown History">
            <div style={{ height: 300 }}>
              <Line data={drawdownChartData} options={chartOptions} />
            </div>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="VaR/CVaR History">
            <div style={{ height: 300 }}>
              <Line data={varChartData} options={chartOptions} />
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Exposure Limits">
            <Table
              dataSource={exposureLimits}
              columns={exposureColumns}
              rowKey="asset"
              pagination={false}
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Risk Profile">
            <div style={{ height: 300 }}>
              <Radar data={riskRadarData} options={{
                ...chartOptions,
                scales: {
                  r: {
                    beginAtZero: true,
                    max: 100,
                  },
                },
              }} />
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card 
            title="Risk Alerts" 
            extra={<Button size="small">Clear All</Button>}
          >
            <Table
              dataSource={alerts}
              columns={alertColumns}
              rowKey="id"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Alert
            message="Risk Management Summary"
            description={
              <div>
                <p>Portfolio is currently operating within acceptable risk parameters.</p>
                <ul>
                  <li>Max Drawdown: {riskMetrics.maxDrawdown}% (Limit: 20%)</li>
                  <li>Sharpe Ratio: {riskMetrics.sharpeRatio} (Target: > 1.5)</li>
                  <li>Total Exposure: ${riskMetrics.totalExposure.toLocaleString()} (Limit: $100,000)</li>
                </ul>
              </div>
            }
            type="info"
            showIcon
          />
        </Col>
      </Row>
    </div>
  );
};

export default RiskManagement;