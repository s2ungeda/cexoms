#!/usr/bin/env python3
"""
OMS Client - Python SDK for Multi-Exchange OMS
"""

import grpc
import time
import argparse
from datetime import datetime
from typing import List, Optional, Dict, Any
import sys
import os

# Add proto path
sys.path.append(os.path.join(os.path.dirname(__file__), '../../proto'))
import oms_pb2
import oms_pb2_grpc


class OMSClient:
    """Python client for OMS gRPC server"""
    
    def __init__(self, server_address: str = "localhost:50051"):
        """Initialize OMS client
        
        Args:
            server_address: gRPC server address
        """
        self.channel = grpc.insecure_channel(server_address)
        self.stub = oms_pb2_grpc.OrderServiceStub(self.channel)
    
    def place_order(self, 
                   symbol: str,
                   side: str,
                   quantity: float,
                   order_type: str = "LIMIT",
                   price: float = 0,
                   exchange: str = "binance",
                   market: str = "spot",
                   account_id: str = "main") -> Dict[str, Any]:
        """Place a new order
        
        Args:
            symbol: Trading symbol (e.g., BTCUSDT)
            side: Order side (BUY or SELL)
            quantity: Order quantity
            order_type: Order type (LIMIT or MARKET)
            price: Order price (required for LIMIT orders)
            exchange: Exchange name
            market: Market type (spot or futures)
            account_id: Account ID
            
        Returns:
            Order response dictionary
        """
        request = oms_pb2.PlaceOrderRequest(
            symbol=symbol,
            side=side,
            order_type=order_type,
            quantity=quantity,
            price=price,
            exchange=exchange,
            market=market,
            account_id=account_id
        )
        
        response = self.stub.PlaceOrder(request)
        return {
            "order_id": response.order_id,
            "exchange_order_id": response.exchange_order_id,
            "status": response.status,
            "created_at": datetime.fromtimestamp(response.created_at)
        }
    
    def cancel_order(self, order_id: str) -> Dict[str, Any]:
        """Cancel an existing order
        
        Args:
            order_id: Order ID to cancel
            
        Returns:
            Cancel response dictionary
        """
        request = oms_pb2.CancelOrderRequest(order_id=order_id)
        response = self.stub.CancelOrder(request)
        
        return {
            "order_id": response.order_id,
            "status": response.status,
            "cancelled_at": datetime.fromtimestamp(response.cancelled_at)
        }
    
    def get_order(self, order_id: str) -> Dict[str, Any]:
        """Get order details
        
        Args:
            order_id: Order ID
            
        Returns:
            Order details dictionary
        """
        request = oms_pb2.GetOrderRequest(order_id=order_id)
        response = self.stub.GetOrder(request)
        
        return self._order_to_dict(response.order)
    
    def list_orders(self, status: str = "", symbol: str = "") -> List[Dict[str, Any]]:
        """List orders with optional filters
        
        Args:
            status: Filter by status (OPEN, FILLED, CANCELLED)
            symbol: Filter by symbol
            
        Returns:
            List of order dictionaries
        """
        request = oms_pb2.ListOrdersRequest(status=status, symbol=symbol)
        response = self.stub.ListOrders(request)
        
        return [self._order_to_dict(order) for order in response.orders]
    
    def get_balance(self, 
                   exchange: str = "binance",
                   market: str = "spot",
                   account_id: str = "main") -> Dict[str, Dict[str, float]]:
        """Get account balance
        
        Args:
            exchange: Exchange name
            market: Market type
            account_id: Account ID
            
        Returns:
            Balance dictionary {asset: {free, locked, total}}
        """
        request = oms_pb2.GetBalanceRequest(
            exchange=exchange,
            market=market,
            account_id=account_id
        )
        response = self.stub.GetBalance(request)
        
        balances = {}
        for balance in response.balances:
            balances[balance.asset] = {
                "free": balance.free,
                "locked": balance.locked,
                "total": balance.free + balance.locked
            }
        
        return balances
    
    def get_positions(self,
                     exchange: str = "binance",
                     account_id: str = "main") -> List[Dict[str, Any]]:
        """Get open positions (futures)
        
        Args:
            exchange: Exchange name
            account_id: Account ID
            
        Returns:
            List of position dictionaries
        """
        request = oms_pb2.GetPositionsRequest(
            exchange=exchange,
            account_id=account_id
        )
        response = self.stub.GetPositions(request)
        
        positions = []
        for pos in response.positions:
            positions.append({
                "symbol": pos.symbol,
                "side": pos.side,
                "size": pos.size,
                "entry_price": pos.entry_price,
                "mark_price": pos.mark_price,
                "unrealized_pnl": pos.unrealized_pnl,
                "pnl_percentage": pos.pnl_percentage,
                "leverage": pos.leverage,
                "margin": pos.margin
            })
        
        return positions
    
    def stream_prices(self, symbols: List[str], callback):
        """Stream real-time prices
        
        Args:
            symbols: List of symbols to stream
            callback: Callback function(price_update)
        """
        request = oms_pb2.StreamPricesRequest(symbols=symbols)
        
        for response in self.stub.StreamPrices(request):
            callback({
                "exchange": response.exchange,
                "symbol": response.symbol,
                "bid_price": response.bid_price,
                "bid_quantity": response.bid_quantity,
                "ask_price": response.ask_price,
                "ask_quantity": response.ask_quantity,
                "last_price": response.last_price,
                "timestamp": datetime.fromtimestamp(response.timestamp)
            })
    
    def stream_orders(self, callback):
        """Stream order updates
        
        Args:
            callback: Callback function(order_update)
        """
        request = oms_pb2.StreamOrdersRequest()
        
        for response in self.stub.StreamOrders(request):
            callback(self._order_to_dict(response.order))
    
    def _order_to_dict(self, order) -> Dict[str, Any]:
        """Convert protobuf order to dictionary"""
        return {
            "order_id": order.order_id,
            "exchange_order_id": order.exchange_order_id,
            "symbol": order.symbol,
            "side": order.side,
            "order_type": order.order_type,
            "quantity": order.quantity,
            "price": order.price,
            "filled_quantity": order.filled_quantity,
            "status": order.status,
            "exchange": order.exchange,
            "market": order.market,
            "account_id": order.account_id,
            "created_at": datetime.fromtimestamp(order.created_at),
            "updated_at": datetime.fromtimestamp(order.updated_at) if order.updated_at else None
        }
    
    def close(self):
        """Close the gRPC channel"""
        self.channel.close()


def main():
    """Example usage of OMS client"""
    parser = argparse.ArgumentParser(description="OMS Python Client")
    parser.add_argument("--server", default="localhost:50051", help="Server address")
    
    subparsers = parser.add_subparsers(dest="command", help="Commands")
    
    # Place order command
    place_parser = subparsers.add_parser("place", help="Place an order")
    place_parser.add_argument("--symbol", required=True, help="Trading symbol")
    place_parser.add_argument("--side", required=True, choices=["BUY", "SELL"], help="Order side")
    place_parser.add_argument("--quantity", type=float, required=True, help="Order quantity")
    place_parser.add_argument("--type", default="LIMIT", choices=["LIMIT", "MARKET"], help="Order type")
    place_parser.add_argument("--price", type=float, default=0, help="Order price")
    place_parser.add_argument("--exchange", default="binance", help="Exchange")
    place_parser.add_argument("--market", default="spot", choices=["spot", "futures"], help="Market type")
    place_parser.add_argument("--account", default="main", help="Account ID")
    
    # Cancel order command
    cancel_parser = subparsers.add_parser("cancel", help="Cancel an order")
    cancel_parser.add_argument("--id", required=True, help="Order ID")
    
    # Get order command
    get_parser = subparsers.add_parser("get", help="Get order details")
    get_parser.add_argument("--id", required=True, help="Order ID")
    
    # List orders command
    list_parser = subparsers.add_parser("list", help="List orders")
    list_parser.add_argument("--status", default="", help="Filter by status")
    list_parser.add_argument("--symbol", default="", help="Filter by symbol")
    
    # Balance command
    balance_parser = subparsers.add_parser("balance", help="Get balance")
    balance_parser.add_argument("--exchange", default="binance", help="Exchange")
    balance_parser.add_argument("--market", default="spot", help="Market type")
    balance_parser.add_argument("--account", default="main", help="Account ID")
    
    # Stream prices command
    stream_parser = subparsers.add_parser("stream", help="Stream prices")
    stream_parser.add_argument("--symbols", nargs="+", default=["BTCUSDT", "ETHUSDT"], help="Symbols to stream")
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        return
    
    # Create client
    client = OMSClient(args.server)
    
    try:
        if args.command == "place":
            result = client.place_order(
                symbol=args.symbol,
                side=args.side,
                quantity=args.quantity,
                order_type=args.type,
                price=args.price,
                exchange=args.exchange,
                market=args.market,
                account_id=args.account
            )
            print(f"Order placed successfully!")
            print(f"Order ID: {result['order_id']}")
            print(f"Status: {result['status']}")
            
        elif args.command == "cancel":
            result = client.cancel_order(args.id)
            print(f"Order cancelled successfully!")
            print(f"Order ID: {result['order_id']}")
            print(f"Status: {result['status']}")
            
        elif args.command == "get":
            order = client.get_order(args.id)
            print(f"Order Details:")
            for key, value in order.items():
                print(f"  {key}: {value}")
                
        elif args.command == "list":
            orders = client.list_orders(status=args.status, symbol=args.symbol)
            print(f"Found {len(orders)} orders:")
            for order in orders:
                print(f"\nOrder {order['order_id']}:")
                print(f"  Symbol: {order['symbol']} | Side: {order['side']} | Type: {order['order_type']}")
                print(f"  Quantity: {order['quantity']} | Filled: {order['filled_quantity']}")
                print(f"  Status: {order['status']}")
                
        elif args.command == "balance":
            balances = client.get_balance(
                exchange=args.exchange,
                market=args.market,
                account_id=args.account
            )
            print(f"Balance for {args.exchange} {args.market} (Account: {args.account}):")
            for asset, balance in balances.items():
                if balance['total'] > 0:
                    print(f"  {asset}: Free={balance['free']:.8f}, Locked={balance['locked']:.8f}, Total={balance['total']:.8f}")
                    
        elif args.command == "stream":
            print(f"Streaming prices for {args.symbols}... (Press Ctrl+C to stop)")
            
            def price_callback(update):
                print(f"[{update['timestamp'].strftime('%H:%M:%S')}] {update['exchange']} {update['symbol']}: "
                      f"Bid=${update['bid_price']:.2f} | Ask=${update['ask_price']:.2f} | Last=${update['last_price']:.2f}")
            
            client.stream_prices(args.symbols, price_callback)
            
    except grpc.RpcError as e:
        print(f"Error: {e.details()}")
    except KeyboardInterrupt:
        print("\nStopped by user")
    finally:
        client.close()


if __name__ == "__main__":
    main()