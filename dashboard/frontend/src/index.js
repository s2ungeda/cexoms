import React from 'react';
import ReactDOM from 'react-dom/client';
import './setupCharts'; // Import Chart.js setup before anything else
import './index.css';
import App from './App';

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);