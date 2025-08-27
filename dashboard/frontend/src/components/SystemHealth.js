import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Progress, Table, Tag, Statistic, Alert, Badge, Space } from 'antd';
import { CheckCircleOutlined, CloseCircleOutlined, SyncOutlined, DatabaseOutlined } from '@ant-design/icons';
import { Line, Bar } from 'react-chartjs-2';
import WebSocketService from '../services/WebSocketService';
import moment from 'moment';

const SystemHealth = () => {
  const [systemMetrics, setSystemMetrics] = useState({
    cpu: 0,
    memory: 0,
    usedMemory: 0,
    diskSpace: 0,
    networkLatency: 0,
    uptime: 0,
    activeConnections: 0,
  });

  const [services, setServices] = useState([]);

  const [performanceMetrics, setPerformanceMetrics] = useState({
    ordersPerSecond: 0,
    messagesPerSecond: 0,
    avgOrderLatency: 0,
    p99OrderLatency: 0,
    errorRate: 0,
  });

  const [cpuHistory, setCpuHistory] = useState([]);
  const [memoryHistory, setMemoryHistory] = useState([]);
  const [latencyHistory, setLatencyHistory] = useState([]);

  useEffect(() => {
    const handleSystemMetrics = (data) => {
      if (data.cpu !== undefined) {
        setSystemMetrics(prev => ({ ...prev, ...data }));
        updateMetricsHistory(data);
      }

      if (data.services) {
        updateServiceStatus(data.services);
      }

      if (data.performance) {
        setPerformanceMetrics(prev => ({ ...prev, ...data.performance }));
      }
    };

    WebSocketService.onSystemMetrics(handleSystemMetrics);

    // Initial state will be populated by real data from WebSocket

    return () => {
      WebSocketService.off('system_metrics', handleSystemMetrics);
    };
  }, []);


  const updateMetricsHistory = (data) => {
    const time = moment().format('HH:mm:ss');

    if (data.cpu !== undefined) {
      setCpuHistory(prev => {
        const updated = [...prev, { time, value: data.cpu }];
        return updated.slice(-60);
      });
    }

    if (data.usedMemory !== undefined) {
      setMemoryHistory(prev => {
        const memoryPercent = (data.usedMemory / data.memory) * 100;
        const updated = [...prev, { time, value: memoryPercent }];
        return updated.slice(-60);
      });
    }

    if (data.networkLatency !== undefined) {
      setLatencyHistory(prev => {
        const updated = [...prev, { time, value: data.networkLatency }];
        return updated.slice(-60);
      });
    }
  };

  const updateServiceStatus = (serviceUpdates) => {
    setServices(prev => {
      const updated = [...prev];
      serviceUpdates.forEach(update => {
        const index = updated.findIndex(s => s.name === update.name);
        if (index !== -1) {
          updated[index] = {
            ...updated[index],
            ...update,
            lastCheck: moment(),
          };
        }
      });
      return updated;
    });
  };

  const formatUptime = (seconds) => {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return `${days}d ${hours}h ${minutes}m`;
  };

  const getServiceStatusIcon = (status) => {
    switch (status) {
      case 'healthy':
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
      case 'warning':
        return <SyncOutlined spin style={{ color: '#faad14' }} />;
      case 'error':
        return <CloseCircleOutlined style={{ color: '#f5222d' }} />;
      default:
        return null;
    }
  };

  // Chart configurations
  const cpuChartData = {
    labels: cpuHistory.map(h => h.time),
    datasets: [{
      label: 'CPU Usage %',
      data: cpuHistory.map(h => h.value),
      borderColor: 'rgb(75, 192, 192)',
      backgroundColor: 'rgba(75, 192, 192, 0.1)',
      tension: 0.4,
    }],
  };

  const memoryChartData = {
    labels: memoryHistory.map(h => h.time),
    datasets: [{
      label: 'Memory Usage %',
      data: memoryHistory.map(h => h.value),
      borderColor: 'rgb(255, 159, 64)',
      backgroundColor: 'rgba(255, 159, 64, 0.1)',
      tension: 0.4,
    }],
  };

  const latencyChartData = {
    labels: ['Orders', 'Market Data', 'Risk Checks', 'Database', 'Cache'],
    datasets: [{
      label: 'Latency (ms)',
      data: [0.5, 1.2, 0.8, 2.1, 0.3],
      backgroundColor: [
        'rgba(255, 99, 132, 0.8)',
        'rgba(54, 162, 235, 0.8)',
        'rgba(255, 206, 86, 0.8)',
        'rgba(75, 192, 192, 0.8)',
        'rgba(153, 102, 255, 0.8)',
      ],
    }],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
    },
    scales: {
      y: {
        beginAtZero: true,
        max: 100,
      },
    },
  };

  const serviceColumns = [
    {
      title: 'Service',
      dataIndex: 'name',
      key: 'name',
      render: (name, record) => (
        <Space>
          {getServiceStatusIcon(record.status)}
          {name}
        </Space>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const color = status === 'healthy' ? 'success' : status === 'warning' ? 'warning' : 'error';
        return <Tag color={color}>{status.toUpperCase()}</Tag>;
      },
    },
    {
      title: 'Latency',
      dataIndex: 'latency',
      key: 'latency',
      render: (latency) => `${latency.toFixed(2)} ms`,
    },
    {
      title: 'Last Check',
      dataIndex: 'lastCheck',
      key: 'lastCheck',
      render: (time) => moment(time).fromNow(),
    },
  ];

  const memoryPercent = (systemMetrics.usedMemory / systemMetrics.memory) * 100;
  const healthyServices = services.filter(s => s.status === 'healthy').length;
  const systemHealthy = healthyServices === services.length;

  return (
    <div>
      <Alert
        message={systemHealthy ? "All Systems Operational" : "System Degraded"}
        description={`${healthyServices}/${services.length} services healthy. System uptime: ${formatUptime(systemMetrics.uptime)}`}
        type={systemHealthy ? "success" : "warning"}
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic
              title="CPU Usage"
              value={systemMetrics.cpu}
              suffix="%"
              valueStyle={{ color: systemMetrics.cpu > 80 ? '#f5222d' : '#3f8600' }}
            />
            <Progress 
              percent={systemMetrics.cpu} 
              strokeColor={systemMetrics.cpu > 80 ? '#f5222d' : '#52c41a'}
              showInfo={false}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Memory Usage"
              value={memoryPercent}
              suffix="%"
              precision={1}
              valueStyle={{ color: memoryPercent > 80 ? '#f5222d' : '#3f8600' }}
            />
            <div style={{ fontSize: '12px', color: '#8c8c8c' }}>
              {systemMetrics.usedMemory} MB / {systemMetrics.memory} MB
            </div>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Disk Usage"
              value={systemMetrics.diskSpace}
              suffix="%"
              valueStyle={{ color: systemMetrics.diskSpace > 90 ? '#f5222d' : '#3f8600' }}
            />
            <Progress 
              percent={systemMetrics.diskSpace} 
              strokeColor={systemMetrics.diskSpace > 90 ? '#f5222d' : '#52c41a'}
              showInfo={false}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Network Latency"
              value={systemMetrics.networkLatency}
              suffix="ms"
              precision={2}
              valueStyle={{ color: systemMetrics.networkLatency > 5 ? '#f5222d' : '#3f8600' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="CPU History">
            <div style={{ height: 200 }}>
              <Line data={cpuChartData} options={chartOptions} />
            </div>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Memory History">
            <div style={{ height: 200 }}>
              <Line data={memoryChartData} options={{
                ...chartOptions,
                scales: { y: { beginAtZero: true, max: 100 } }
              }} />
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Service Health">
            <Table
              dataSource={services}
              columns={serviceColumns}
              rowKey="name"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Component Latency">
            <div style={{ height: 250 }}>
              <Bar data={latencyChartData} options={{
                ...chartOptions,
                scales: { y: { beginAtZero: true, max: undefined } }
              }} />
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card title="Performance Metrics">
            <Row gutter={16}>
              <Col span={4}>
                <Statistic
                  title="Orders/sec"
                  value={performanceMetrics.ordersPerSecond}
                  valueStyle={{ color: '#1890ff' }}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="Messages/sec"
                  value={performanceMetrics.messagesPerSecond}
                  valueStyle={{ color: '#1890ff' }}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="Avg Order Latency"
                  value={performanceMetrics.avgOrderLatency}
                  suffix="ms"
                  precision={2}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="P99 Latency"
                  value={performanceMetrics.p99OrderLatency}
                  suffix="ms"
                  precision={2}
                  valueStyle={{ color: performanceMetrics.p99OrderLatency > 5 ? '#faad14' : '#52c41a' }}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="Error Rate"
                  value={performanceMetrics.errorRate}
                  suffix="%"
                  precision={3}
                  valueStyle={{ color: performanceMetrics.errorRate > 0.1 ? '#f5222d' : '#52c41a' }}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="Active Connections"
                  value={systemMetrics.activeConnections || 0}
                  prefix={<DatabaseOutlined />}
                />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default SystemHealth;