import React, { useState, useEffect } from 'react';
import { Layout, Menu, theme } from 'antd';
import {
  DashboardOutlined,
  ShoppingCartOutlined,
  LineChartOutlined,
  SafetyOutlined,
  SettingOutlined,
  BarChartOutlined,
} from '@ant-design/icons';
import { BrowserRouter as Router, Routes, Route, Link } from 'react-router-dom';
import Overview from './components/Overview';
import Orders from './components/Orders';
import Positions from './components/Positions';
import MarketData from './components/MarketData';
import RiskManagement from './components/RiskManagement';
import SystemHealth from './components/SystemHealth';
import WebSocketService from './services/WebSocketService';
import './App.css';

const { Header, Sider, Content } = Layout;

function App() {
  const [collapsed, setCollapsed] = useState(false);
  const [wsConnected, setWsConnected] = useState(false);
  const {
    token: { colorBgContainer },
  } = theme.useToken();

  useEffect(() => {
    // Initialize WebSocket connection
    WebSocketService.connect('ws://localhost:8080/ws');
    
    WebSocketService.onConnect(() => {
      setWsConnected(true);
      // Subscribe to all streams
      WebSocketService.subscribe(['orders', 'positions', 'market', 'system', 'risk']);
    });

    WebSocketService.onDisconnect(() => {
      setWsConnected(false);
    });

    return () => {
      WebSocketService.disconnect();
    };
  }, []);

  return (
    <Router>
      <Layout style={{ minHeight: '100vh' }}>
        <Sider trigger={null} collapsible collapsed={collapsed}>
          <div className="logo">
            <h2 style={{ color: 'white', padding: '16px' }}>
              {collapsed ? 'OMS' : 'mExOms'}
            </h2>
          </div>
          <Menu
            theme="dark"
            mode="inline"
            defaultSelectedKeys={['1']}
            items={[
              {
                key: '1',
                icon: <DashboardOutlined />,
                label: <Link to="/">Overview</Link>,
              },
              {
                key: '2',
                icon: <ShoppingCartOutlined />,
                label: <Link to="/orders">Orders</Link>,
              },
              {
                key: '3',
                icon: <BarChartOutlined />,
                label: <Link to="/positions">Positions</Link>,
              },
              {
                key: '4',
                icon: <LineChartOutlined />,
                label: <Link to="/market">Market Data</Link>,
              },
              {
                key: '5',
                icon: <SafetyOutlined />,
                label: <Link to="/risk">Risk Management</Link>,
              },
              {
                key: '6',
                icon: <SettingOutlined />,
                label: <Link to="/system">System Health</Link>,
              },
            ]}
          />
        </Sider>
        <Layout>
          <Header style={{ 
            padding: 0, 
            background: colorBgContainer,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between'
          }}>
            <h1 style={{ margin: '0 24px' }}>mExOms Monitoring Dashboard</h1>
            <div style={{ marginRight: '24px' }}>
              <span style={{ 
                marginRight: '16px',
                color: wsConnected ? 'green' : 'red' 
              }}>
                {wsConnected ? '● Connected' : '○ Disconnected'}
              </span>
            </div>
          </Header>
          <Content
            style={{
              margin: '24px 16px',
              padding: 24,
              minHeight: 280,
              background: colorBgContainer,
            }}
          >
            <Routes>
              <Route path="/" element={<Overview />} />
              <Route path="/orders" element={<Orders />} />
              <Route path="/positions" element={<Positions />} />
              <Route path="/market" element={<MarketData />} />
              <Route path="/risk" element={<RiskManagement />} />
              <Route path="/system" element={<SystemHealth />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </Router>
  );
}

export default App;