import React, { useState, useEffect } from 'react';
import { Table, Card, Tag, Button, Space, Row, Col, Statistic, Input, Select } from 'antd';
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons';
import WebSocketService from '../services/WebSocketService';
import moment from 'moment';

const { Option } = Select;

const Orders = () => {
  const [orders, setOrders] = useState([]);
  const [filteredOrders, setFilteredOrders] = useState([]);
  const [activeCount, setActiveCount] = useState(0);
  const [filledCount, setFilledCount] = useState(0);
  const [cancelledCount, setCancelledCount] = useState(0);
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    // WebSocket listener for order updates
    const handleOrderUpdate = (data) => {
      setOrders(prev => {
        const existingIndex = prev.findIndex(order => order.orderId === data.orderId);
        
        if (existingIndex !== -1) {
          // Update existing order
          const updated = [...prev];
          updated[existingIndex] = { ...updated[existingIndex], ...data, lastUpdate: Date.now() };
          return updated;
        } else {
          // Add new order
          return [{ ...data, lastUpdate: Date.now() }, ...prev];
        }
      });

      // Update counters
      updateCounters(data);
    };

    WebSocketService.onOrderUpdate(handleOrderUpdate);

    // Initial orders will come from real OMS via WebSocket

    return () => {
      WebSocketService.off('order_update', handleOrderUpdate);
    };
  }, []);

  useEffect(() => {
    // Filter orders based on search and status
    let filtered = orders;

    if (searchText) {
      filtered = filtered.filter(order =>
        order.symbol.toLowerCase().includes(searchText.toLowerCase()) ||
        order.orderId.toLowerCase().includes(searchText.toLowerCase())
      );
    }

    if (statusFilter !== 'all') {
      filtered = filtered.filter(order => order.status === statusFilter);
    }

    setFilteredOrders(filtered);
  }, [orders, searchText, statusFilter]);


  const updateCounters = (order) => {
    if (order.status === 'NEW' || order.status === 'PARTIALLY_FILLED') {
      setActiveCount(prev => prev + 1);
    } else if (order.status === 'FILLED') {
      setFilledCount(prev => prev + 1);
      setActiveCount(prev => Math.max(0, prev - 1));
    } else if (order.status === 'CANCELLED') {
      setCancelledCount(prev => prev + 1);
      setActiveCount(prev => Math.max(0, prev - 1));
    }
  };

  const handleRefresh = () => {
    setLoading(true);
    // Simulate refresh
    setTimeout(() => {
      setLoading(false);
    }, 1000);
  };

  const getStatusColor = (status) => {
    const statusColors = {
      NEW: 'blue',
      PARTIALLY_FILLED: 'orange',
      FILLED: 'green',
      CANCELLED: 'red',
      REJECTED: 'red',
      EXPIRED: 'grey',
    };
    return statusColors[status] || 'default';
  };

  const columns = [
    {
      title: 'Order ID',
      dataIndex: 'orderId',
      key: 'orderId',
      fixed: 'left',
      width: 120,
    },
    {
      title: 'Time',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 150,
      render: (timestamp) => moment(timestamp).format('MM/DD HH:mm:ss'),
      sorter: (a, b) => new Date(a.timestamp) - new Date(b.timestamp),
    },
    {
      title: 'Symbol',
      dataIndex: 'symbol',
      key: 'symbol',
      width: 100,
      filters: [...new Set(orders.map(o => o.symbol))].map(symbol => ({ text: symbol, value: symbol })),
      onFilter: (value, record) => record.symbol === value,
    },
    {
      title: 'Side',
      dataIndex: 'side',
      key: 'side',
      width: 80,
      render: (side) => (
        <Tag color={side === 'BUY' ? 'green' : 'red'}>{side}</Tag>
      ),
      filters: [
        { text: 'BUY', value: 'BUY' },
        { text: 'SELL', value: 'SELL' },
      ],
      onFilter: (value, record) => record.side === value,
    },
    {
      title: 'Type',
      dataIndex: 'type',
      key: 'type',
      width: 100,
    },
    {
      title: 'Quantity',
      dataIndex: 'quantity',
      key: 'quantity',
      width: 100,
      align: 'right',
    },
    {
      title: 'Price',
      dataIndex: 'price',
      key: 'price',
      width: 100,
      align: 'right',
      render: (price) => `$${price?.toLocaleString() || '-'}`,
    },
    {
      title: 'Filled',
      key: 'filled',
      width: 120,
      render: (record) => {
        const fillPercentage = record.filledQuantity / record.quantity * 100;
        return (
          <span>
            {record.filledQuantity}/{record.quantity}
            <br />
            <small>({fillPercentage.toFixed(0)}%)</small>
          </span>
        );
      },
    },
    {
      title: 'Avg Price',
      dataIndex: 'avgPrice',
      key: 'avgPrice',
      width: 100,
      align: 'right',
      render: (price) => price ? `$${price.toLocaleString()}` : '-',
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status) => (
        <Tag color={getStatusColor(status)}>{status}</Tag>
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
        <Space size="small">
          {(record.status === 'NEW' || record.status === 'PARTIALLY_FILLED') && (
            <Button size="small" danger>Cancel</Button>
          )}
        </Space>
      ),
    },
  ];

  const rowClassName = (record) => {
    if (record.lastUpdate && Date.now() - record.lastUpdate < 1000) {
      return 'realtime-update';
    }
    return `order-row-${record.status.toLowerCase()}`;
  };

  return (
    <div>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic
              title="Active Orders"
              value={activeCount}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Filled Today"
              value={filledCount}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Cancelled"
              value={cancelledCount}
              valueStyle={{ color: '#f5222d' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Total Volume"
              value={125000}
              prefix="$"
              precision={0}
            />
          </Card>
        </Col>
      </Row>

      <Card 
        style={{ marginTop: 16 }}
        title="Order Book"
        extra={
          <Space>
            <Input
              placeholder="Search orders"
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={e => setSearchText(e.target.value)}
              style={{ width: 200 }}
            />
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              style={{ width: 120 }}
            >
              <Option value="all">All Status</Option>
              <Option value="NEW">Active</Option>
              <Option value="FILLED">Filled</Option>
              <Option value="CANCELLED">Cancelled</Option>
            </Select>
            <Button
              icon={<ReloadOutlined />}
              onClick={handleRefresh}
              loading={loading}
            >
              Refresh
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={filteredOrders}
          rowKey="orderId"
          rowClassName={rowClassName}
          scroll={{ x: 1300 }}
          pagination={{
            pageSize: 20,
            showSizeChanger: true,
            showTotal: (total) => `Total ${total} orders`,
          }}
        />
      </Card>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="Order Type Distribution">
            <Row gutter={16}>
              <Col span={8}>
                <Statistic title="Market" value={45} suffix="%" />
              </Col>
              <Col span={8}>
                <Statistic title="Limit" value={40} suffix="%" />
              </Col>
              <Col span={8}>
                <Statistic title="Stop" value={15} suffix="%" />
              </Col>
            </Row>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Average Execution Time">
            <Row gutter={16}>
              <Col span={8}>
                <Statistic title="Market" value={125} suffix="ms" />
              </Col>
              <Col span={8}>
                <Statistic title="Limit" value={580} suffix="ms" />
              </Col>
              <Col span={8}>
                <Statistic title="Overall" value={352} suffix="ms" />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default Orders;