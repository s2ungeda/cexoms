import React, { useState, useEffect } from 'react';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  TimeScale,
  Tooltip,
  Legend,
} from 'chart.js';
import { Chart } from 'react-chartjs-2';
import 'chartjs-adapter-date-fns';
import { CandlestickController, CandlestickElement } from 'chartjs-chart-financial';
import { Radio, Card } from 'antd';
import WebSocketService from '../services/WebSocketService';

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  TimeScale,
  Tooltip,
  Legend,
  CandlestickController,
  CandlestickElement
);

const CandlestickChart = ({ symbol }) => {
  const [interval, setInterval] = useState('1m');
  const [klineData, setKlineData] = useState({});

  useEffect(() => {
    const handleKlineUpdate = (data) => {
      // Parse data if it's a string
      const parsedData = typeof data === 'string' ? JSON.parse(data) : data;
      
      if (parsedData.symbol === symbol && parsedData.interval === interval) {
        setKlineData(prev => {
          const key = `${parsedData.symbol}_${parsedData.interval}`;
          const existing = prev[key] || [];
          
          // Find if we need to update existing candle or add new one
          const existingIndex = existing.findIndex(k => k.x === parsedData.open_time);
          
          const candle = {
            x: parsedData.open_time,
            o: parsedData.open,
            h: parsedData.high,
            l: parsedData.low,
            c: parsedData.close,
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

  const getChartData = () => {
    const key = `${symbol}_${interval}`;
    const candles = klineData[key] || [];
    
    return {
      datasets: [{
        label: `${symbol} ${interval}`,
        data: candles,
        borderColor: ctx => {
          const candle = ctx.raw;
          return candle && candle.c >= candle.o ? '#52c41a' : '#f5222d';
        },
        backgroundColor: ctx => {
          const candle = ctx.raw;
          return candle && candle.c >= candle.o ? 'rgba(82, 196, 26, 0.8)' : 'rgba(245, 34, 45, 0.8)';
        },
        borderWidth: 1,
      }],
    };
  };

  const options = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        callbacks: {
          label: function(context) {
            const candle = context.raw;
            if (!candle) return '';
            
            return [
              `Open: $${candle.o.toFixed(2)}`,
              `High: $${candle.h.toFixed(2)}`,
              `Low: $${candle.l.toFixed(2)}`,
              `Close: $${candle.c.toFixed(2)}`,
            ];
          }
        }
      }
    },
    scales: {
      x: {
        type: 'time',
        time: {
          unit: interval === '1m' ? 'minute' : 
                interval === '5m' ? 'minute' :
                interval === '1h' ? 'hour' : 'day',
          displayFormats: {
            minute: 'HH:mm',
            hour: 'HH:mm',
            day: 'MMM dd',
          }
        }
      },
      y: {
        position: 'right',
      }
    }
  };

  return (
    <Card 
      title={`${symbol} Candlestick Chart`}
      extra={
        <Radio.Group value={interval} onChange={e => setInterval(e.target.value)}>
          <Radio.Button value="1m">1m</Radio.Button>
          <Radio.Button value="5m">5m</Radio.Button>
          <Radio.Button value="1h">1h</Radio.Button>
          <Radio.Button value="1d">1d</Radio.Button>
        </Radio.Group>
      }
    >
      <div style={{ height: '400px' }}>
        <Chart type='candlestick' data={getChartData()} options={options} />
      </div>
    </Card>
  );
};

export default CandlestickChart;