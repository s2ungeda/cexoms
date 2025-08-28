import React, { useState, useEffect } from 'react';
import { Card, Radio, Empty, Spin } from 'antd';
import WebSocketService from '../services/WebSocketService';

const SimpleCandlestickChart = ({ symbol }) => {
  const [interval, setInterval] = useState('1m');
  const [klineData, setKlineData] = useState({});
  const [loading, setLoading] = useState(true);

  // Fetch historical kline data
  const fetchHistoricalKlines = async (symbol, interval) => {
    setLoading(true);
    try {
      const limit = interval === '1m' ? 120 : 
                   interval === '5m' ? 120 :
                   interval === '1h' ? 96 : 
                   interval === '1d' ? 60 : 120; // Doubled for better view
      
      const response = await fetch(
        `https://api.binance.com/api/v3/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`
      );
      
      if (!response.ok) {
        throw new Error('Failed to fetch klines');
      }
      
      const data = await response.json();
      
      // Convert Binance kline format to our format
      const formattedKlines = data.map(kline => ({
        time: kline[0], // Open time
        open: parseFloat(kline[1]),
        high: parseFloat(kline[2]),
        low: parseFloat(kline[3]),
        close: parseFloat(kline[4]),
        volume: parseFloat(kline[5]),
      }));
      
      const key = `${symbol}_${interval}`;
      setKlineData(prev => ({
        ...prev,
        [key]: formattedKlines
      }));
    } catch (error) {
      console.error('Failed to fetch historical klines:', error);
    } finally {
      setLoading(false);
    }
  };

  // Fetch historical data when symbol or interval changes
  useEffect(() => {
    if (symbol && interval) {
      fetchHistoricalKlines(symbol, interval);
    }
  }, [symbol, interval]);

  useEffect(() => {
    const handleKlineUpdate = (data) => {
      // Parse data if it's a string
      const parsedData = typeof data === 'string' ? JSON.parse(data) : data;
      
      if (parsedData.symbol === symbol && parsedData.interval === interval) {
        setKlineData(prev => {
          const key = `${parsedData.symbol}_${parsedData.interval}`;
          const existing = prev[key] || [];
          
          // Find if we need to update existing candle or add new one
          const existingIndex = existing.findIndex(k => k.time === parsedData.open_time);
          
          const candle = {
            time: parsedData.open_time,
            open: parsedData.open,
            high: parsedData.high,
            low: parsedData.low,
            close: parsedData.close,
            volume: parsedData.volume,
          };
          
          let newData;
          if (existingIndex >= 0) {
            // Update existing candle
            newData = [...existing];
            newData[existingIndex] = candle;
          } else {
            // Add new candle
            newData = [...existing, candle].slice(-100); // Keep last 100 candles
          }
          
          return {
            ...prev,
            [key]: newData
          };
        });
      }
    };

    WebSocketService.on('kline_update', handleKlineUpdate);

    return () => {
      WebSocketService.off('kline_update', handleKlineUpdate);
    };
  }, [symbol, interval]);

  const formatTime = (timestamp) => {
    const date = new Date(timestamp);
    if (interval === '1m' || interval === '5m') {
      return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
    } else if (interval === '1h') {
      return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
    } else {
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }
  };

  const key = `${symbol}_${interval}`;
  const candles = klineData[key] || [];
  const hasData = candles.length > 0;

  // Calculate chart dimensions
  const chartHeight = 400;
  const chartWidth = '100%';
  const candleWidth = 8;
  const candleSpacing = 2;
  const margin = { top: 20, bottom: 30, left: 60, right: 100 };
  const plotHeight = chartHeight - margin.top - margin.bottom;
  
  // Find price range with proper validation
  let maxPrice = 0;
  let minPrice = Infinity;
  
  if (candles.length > 0) {
    candles.forEach(c => {
      if (c.high > maxPrice) maxPrice = c.high;
      if (c.low < minPrice) minPrice = c.low;
    });
  }
  
  // Add padding to price range (5%)
  const priceRange = maxPrice - minPrice;
  const padding = priceRange * 0.05 || maxPrice * 0.01; // If range is 0, use 1% of max
  maxPrice += padding;
  minPrice -= padding;
  
  // Ensure minimum range
  if (priceRange < 0.01) {
    const midPrice = (maxPrice + minPrice) / 2;
    maxPrice = midPrice * 1.001;
    minPrice = midPrice * 0.999;
  }
  
  // Debug log
  if (candles.length > 0) {
    console.log(`Chart for ${symbol} ${interval}:`, {
      candles: candles.length,
      minPrice,
      maxPrice,
      priceRange: maxPrice - minPrice,
      firstCandle: candles[0],
      lastCandle: candles[candles.length - 1]
    });
  }
  
  // Scale helper - inverted Y axis
  const scaleY = (price) => {
    const normalizedPrice = (price - minPrice) / (maxPrice - minPrice);
    return margin.top + (1 - normalizedPrice) * plotHeight;
  };

  return (
    <Card 
      title={`${symbol} Candlestick Chart`}
      extra={
        <Radio.Group value={interval} onChange={e => setInterval(e.target.value)} size="small">
          <Radio.Button value="1m">1m</Radio.Button>
          <Radio.Button value="5m">5m</Radio.Button>
          <Radio.Button value="1h">1h</Radio.Button>
          <Radio.Button value="1d">1d</Radio.Button>
        </Radio.Group>
      }
    >
      {loading ? (
        <div style={{ height: chartHeight, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
          <Spin size="large" tip="Loading historical data..." />
        </div>
      ) : !hasData ? (
        <Empty description="No kline data available" style={{ height: chartHeight }} />
      ) : (
        <div style={{ position: 'relative', width: '100%', height: chartHeight }}>
          <svg width="100%" height={chartHeight} style={{ display: 'block' }}>
            {/* Candlesticks */}
            {candles.map((candle, index) => {
              const x = index * (candleWidth + candleSpacing) + margin.left;
              const isGreen = candle.close >= candle.open;
              const color = isGreen ? '#52c41a' : '#f5222d';
              const bodyTop = scaleY(Math.max(candle.open, candle.close));
              const bodyBottom = scaleY(Math.min(candle.open, candle.close));
              const bodyHeight = Math.max(bodyBottom - bodyTop, 1);
              
              return (
                <g key={index}>
                  {/* High-Low line */}
                  <line
                    x1={x + candleWidth / 2}
                    y1={scaleY(candle.high)}
                    x2={x + candleWidth / 2}
                    y2={scaleY(candle.low)}
                    stroke={color}
                    strokeWidth={1}
                  />
                  {/* Body */}
                  <rect
                    x={x}
                    y={bodyTop}
                    width={candleWidth}
                    height={bodyHeight}
                    fill={color}
                    fillOpacity={0.8}
                  />
                  {/* Time label every 10 candles */}
                  {index % 10 === 0 && (
                    <text
                      x={x + candleWidth / 2}
                      y={chartHeight - 5}
                      fill="#999"
                      fontSize="10"
                      textAnchor="middle"
                    >
                      {formatTime(candle.time)}
                    </text>
                  )}
                </g>
              );
            })}
            
            {/* Price grid lines - drawn after candlesticks to be on top */}
            {[0, 0.25, 0.5, 0.75, 1].map((ratio) => {
              const price = minPrice + ratio * (maxPrice - minPrice);
              const y = scaleY(price);
              return (
                <g key={ratio}>
                  <line
                    x1={margin.left}
                    y1={y}
                    x2="90%"
                    y2={y}
                    stroke="#f0f0f0"
                    strokeDasharray="2,2"
                  />
                </g>
              );
            })}
          </svg>
          
          {/* Y-axis price labels on the right side */}
          <div style={{
            position: 'absolute',
            right: 0,
            top: 0,
            width: margin.right - 10,
            height: chartHeight,
            backgroundColor: 'white',
            borderLeft: '1px solid #f0f0f0'
          }}>
            {[0, 0.25, 0.5, 0.75, 1].map((ratio) => {
              const price = minPrice + ratio * (maxPrice - minPrice);
              const y = scaleY(price);
              return (
                <div
                  key={ratio}
                  style={{
                    position: 'absolute',
                    right: 5,
                    top: y - 10,
                    height: 20,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'flex-end',
                    fontSize: 12,
                    color: '#666'
                  }}
                >
                  ${price.toFixed(symbol.includes('BTC') ? 2 : 4)}
                </div>
              );
            })}
          </div>
          
          {/* Last price indicator */}
          {candles.length > 0 && (
            <div
              style={{
                position: 'absolute',
                right: margin.right - 5,
                top: scaleY(candles[candles.length - 1].close) - 10,
                backgroundColor: candles[candles.length - 1].close >= candles[candles.length - 1].open ? '#52c41a' : '#f5222d',
                color: 'white',
                padding: '2px 8px',
                borderRadius: 2,
                fontSize: 12,
                zIndex: 20,
              }}
            >
              ${candles[candles.length - 1].close.toFixed(symbol.includes('BTC') ? 2 : 4)}
            </div>
          )}
        </div>
      )}
    </Card>
  );
};

export default SimpleCandlestickChart;