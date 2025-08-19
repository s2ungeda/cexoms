# OMS Windows Client Guide

## Overview

The OMS Windows Client provides a graphical user interface (GUI) for trading on multiple cryptocurrency exchanges. It connects to the OMS server via REST API.

## Features

- **Trading Interface**: Place market and limit orders
- **Order Management**: View and cancel active orders
- **Balance Display**: Check account balances
- **Real-time Market Data**: Live price updates
- **Activity Log**: Track all operations

## Installation

### Option 1: Run Python Script

1. Install Python 3.8 or higher
2. Install dependencies:
   ```cmd
   cd clients\windows
   pip install -r requirements.txt
   ```
3. Run the client:
   ```cmd
   python oms-client.py
   ```

### Option 2: Use Executable (Windows Only)

1. Navigate to `clients\windows`
2. Run the build script:
   ```cmd
   build.bat
   ```
3. Find the executable in `dist\OMS Trading Client.exe`
4. Double-click to run

## Configuration

### Server Connection

By default, the client connects to `http://localhost:8080`. To connect to a different server:

1. Edit `oms-client.py`
2. Change the `base_url` parameter:
   ```python
   self.client = OMSClient(base_url="http://your-server:8080/api/v1")
   ```

### Supported Exchanges

- Binance Spot
- Binance Futures

## Usage Guide

### 1. Trading Tab

#### Placing Orders

1. Select symbol (BTCUSDT, ETHUSDT, etc.)
2. Choose side (Buy/Sell)
3. Select order type:
   - **Limit**: Specify price and quantity
   - **Market**: Specify quantity only
4. Enter quantity
5. For limit orders, enter price
6. Select market (Spot/Futures)
7. Click "Place Order"

#### Current Prices

The bottom panel shows real-time prices:
- Bid price and quantity
- Ask price and quantity
- Last traded price
- 24h change percentage

### 2. Orders Tab

#### View Orders

- All active and recent orders are displayed
- Shows order details: symbol, side, type, quantity, price, status, time

#### Cancel Orders

1. Select an order from the list
2. Click "Cancel Selected"
3. Confirm cancellation

#### Refresh Orders

Click "Refresh" to update the orders list

### 3. Balance Tab

#### View Balance

1. Select market type (Spot/Futures)
2. Click "Refresh"
3. View available assets:
   - Free: Available for trading
   - Locked: In active orders
   - Total: Free + Locked

### 4. Market Data Tab

Shows comprehensive market information:
- Real-time bid/ask prices
- 24h volume
- 24h high/low
- Price change percentage

### 5. Log Tab

- View all client activities
- Track order placements and cancellations
- Monitor errors and system messages
- Click "Clear Log" to reset

## Keyboard Shortcuts

- `F5`: Refresh current view
- `Ctrl+O`: Focus on order form
- `Ctrl+L`: Clear log
- `Esc`: Cancel current operation

## Troubleshooting

### Connection Issues

**Problem**: Cannot connect to server
**Solution**: 
1. Verify OMS server is running
2. Check firewall settings
3. Ensure correct server address
4. Try: `http://localhost:8080/api/v1/health`

### Order Failures

**Problem**: Orders failing
**Solution**:
1. Check account balance
2. Verify minimum order size
3. Ensure API keys are configured on server
4. Check symbol format (BTCUSDT not BTC/USDT)

### Price Updates

**Problem**: Prices not updating
**Solution**:
1. Check internet connection
2. Verify server is receiving market data
3. Restart the client

## Error Messages

### Common Errors

1. **"Failed to place order: Invalid request body"**
   - Check all required fields are filled
   - Verify quantity and price are numbers

2. **"Failed to connect to server"**
   - Server may be offline
   - Check network connection
   - Verify server address

3. **"Insufficient balance"**
   - Not enough funds in account
   - Check the Balance tab

4. **"Minimum order size not met"**
   - Order quantity too small
   - Check exchange requirements

## Best Practices

1. **Start Small**: Test with minimum order sizes first
2. **Use Limit Orders**: More control over execution price
3. **Monitor Orders**: Regularly check order status
4. **Check Balance**: Ensure sufficient funds before trading
5. **Review Logs**: Check activity log for issues

## Security

1. **API Keys**: Stored securely on server, not in client
2. **HTTPS**: Use HTTPS in production
3. **Authentication**: Implement user authentication for production use
4. **Network**: Use VPN on public networks

## System Requirements

### Minimum Requirements
- Windows 7 or higher
- 2GB RAM
- 100MB disk space
- Internet connection

### Recommended
- Windows 10/11
- 4GB RAM
- SSD storage
- Stable broadband connection

## Support

For issues or questions:
1. Check the Log tab for error details
2. Verify server status
3. Review this guide
4. Contact system administrator

## Updates

To update the client:
1. Download latest version
2. Replace executable or Python script
3. Restart client

## Advanced Configuration

### Custom Symbols

Add new trading pairs by editing the symbol list in `oms-client.py`:
```python
self.symbol_combo = ttk.Combobox(form_frame, textvariable=self.symbol_var, 
                                values=["BTCUSDT", "ETHUSDT", "XRPUSDT", "BNBUSDT", "SOLUSDT"])
```

### Refresh Intervals

Adjust price update frequency:
```python
time.sleep(5)  # Change from 5 seconds to desired interval
```

### Color Themes

Modify the GUI theme:
```python
style.theme_use('clam')  # Options: 'clam', 'alt', 'default', 'classic'
```