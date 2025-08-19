#!/usr/bin/env python3
"""
OMS Windows Client - GUI Application for Windows
"""

import tkinter as tk
from tkinter import ttk, messagebox, scrolledtext
import requests
import json
import threading
import time
from datetime import datetime
import queue

class OMSClient:
    def __init__(self, base_url="http://localhost:8080/api/v1"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({
            'Content-Type': 'application/json',
            'Accept': 'application/json'
        })

    def place_order(self, **kwargs):
        """Place a new order"""
        try:
            response = self.session.post(f"{self.base_url}/orders", json=kwargs)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            raise Exception(f"Failed to place order: {e}")

    def cancel_order(self, order_id):
        """Cancel an order"""
        try:
            response = self.session.delete(f"{self.base_url}/orders/{order_id}")
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            raise Exception(f"Failed to cancel order: {e}")

    def get_balance(self, exchange="binance", market="spot", account_id="main"):
        """Get account balance"""
        try:
            params = {
                'exchange': exchange,
                'market': market,
                'account_id': account_id
            }
            response = self.session.get(f"{self.base_url}/balance", params=params)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            raise Exception(f"Failed to get balance: {e}")

    def get_prices(self, symbols=None):
        """Get current prices"""
        try:
            params = {}
            if symbols:
                params['symbol'] = symbols
            response = self.session.get(f"{self.base_url}/prices", params=params)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            raise Exception(f"Failed to get prices: {e}")

    def list_orders(self, status=None, symbol=None):
        """List orders"""
        try:
            params = {}
            if status:
                params['status'] = status
            if symbol:
                params['symbol'] = symbol
            response = self.session.get(f"{self.base_url}/orders", params=params)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            raise Exception(f"Failed to list orders: {e}")

class OMSClientGUI:
    def __init__(self, root):
        self.root = root
        self.root.title("OMS Trading Client")
        self.root.geometry("1200x800")
        
        # Initialize client
        self.client = OMSClient()
        
        # Message queue for thread communication
        self.message_queue = queue.Queue()
        
        # Create UI
        self.create_widgets()
        
        # Start background threads
        self.price_update_thread = threading.Thread(target=self.update_prices_loop, daemon=True)
        self.price_update_thread.start()
        
        # Process messages from queue
        self.process_queue()

    def create_widgets(self):
        # Create notebook for tabs
        notebook = ttk.Notebook(self.root)
        notebook.pack(fill='both', expand=True, padx=5, pady=5)
        
        # Trading tab
        self.trading_frame = ttk.Frame(notebook)
        notebook.add(self.trading_frame, text='Trading')
        self.create_trading_tab()
        
        # Orders tab
        self.orders_frame = ttk.Frame(notebook)
        notebook.add(self.orders_frame, text='Orders')
        self.create_orders_tab()
        
        # Balance tab
        self.balance_frame = ttk.Frame(notebook)
        notebook.add(self.balance_frame, text='Balance')
        self.create_balance_tab()
        
        # Market Data tab
        self.market_frame = ttk.Frame(notebook)
        notebook.add(self.market_frame, text='Market Data')
        self.create_market_tab()
        
        # Log tab
        self.log_frame = ttk.Frame(notebook)
        notebook.add(self.log_frame, text='Log')
        self.create_log_tab()

    def create_trading_tab(self):
        # Order form
        form_frame = ttk.LabelFrame(self.trading_frame, text="Place Order", padding=10)
        form_frame.pack(fill='x', padx=10, pady=10)
        
        # Symbol
        ttk.Label(form_frame, text="Symbol:").grid(row=0, column=0, sticky='w', padx=5, pady=5)
        self.symbol_var = tk.StringVar(value="BTCUSDT")
        self.symbol_combo = ttk.Combobox(form_frame, textvariable=self.symbol_var, 
                                        values=["BTCUSDT", "ETHUSDT", "XRPUSDT", "BNBUSDT"])
        self.symbol_combo.grid(row=0, column=1, padx=5, pady=5, sticky='ew')
        
        # Side
        ttk.Label(form_frame, text="Side:").grid(row=0, column=2, sticky='w', padx=5, pady=5)
        self.side_var = tk.StringVar(value="BUY")
        side_frame = ttk.Frame(form_frame)
        side_frame.grid(row=0, column=3, padx=5, pady=5)
        ttk.Radiobutton(side_frame, text="Buy", variable=self.side_var, value="BUY").pack(side='left')
        ttk.Radiobutton(side_frame, text="Sell", variable=self.side_var, value="SELL").pack(side='left')
        
        # Order Type
        ttk.Label(form_frame, text="Type:").grid(row=1, column=0, sticky='w', padx=5, pady=5)
        self.order_type_var = tk.StringVar(value="LIMIT")
        type_frame = ttk.Frame(form_frame)
        type_frame.grid(row=1, column=1, padx=5, pady=5)
        ttk.Radiobutton(type_frame, text="Limit", variable=self.order_type_var, 
                       value="LIMIT", command=self.toggle_price_field).pack(side='left')
        ttk.Radiobutton(type_frame, text="Market", variable=self.order_type_var, 
                       value="MARKET", command=self.toggle_price_field).pack(side='left')
        
        # Quantity
        ttk.Label(form_frame, text="Quantity:").grid(row=1, column=2, sticky='w', padx=5, pady=5)
        self.quantity_var = tk.StringVar(value="0.001")
        self.quantity_entry = ttk.Entry(form_frame, textvariable=self.quantity_var)
        self.quantity_entry.grid(row=1, column=3, padx=5, pady=5, sticky='ew')
        
        # Price
        ttk.Label(form_frame, text="Price:").grid(row=2, column=0, sticky='w', padx=5, pady=5)
        self.price_var = tk.StringVar(value="115000")
        self.price_entry = ttk.Entry(form_frame, textvariable=self.price_var)
        self.price_entry.grid(row=2, column=1, padx=5, pady=5, sticky='ew')
        
        # Market
        ttk.Label(form_frame, text="Market:").grid(row=2, column=2, sticky='w', padx=5, pady=5)
        self.market_var = tk.StringVar(value="spot")
        market_combo = ttk.Combobox(form_frame, textvariable=self.market_var, 
                                   values=["spot", "futures"], state='readonly')
        market_combo.grid(row=2, column=3, padx=5, pady=5, sticky='ew')
        
        # Buttons
        button_frame = ttk.Frame(form_frame)
        button_frame.grid(row=3, column=0, columnspan=4, pady=10)
        
        self.place_order_btn = ttk.Button(button_frame, text="Place Order", 
                                         command=self.place_order, style='Accent.TButton')
        self.place_order_btn.pack(side='left', padx=5)
        
        ttk.Button(button_frame, text="Clear", command=self.clear_form).pack(side='left', padx=5)
        
        # Configure grid weights
        form_frame.columnconfigure(1, weight=1)
        form_frame.columnconfigure(3, weight=1)
        
        # Current prices display
        prices_frame = ttk.LabelFrame(self.trading_frame, text="Current Prices", padding=10)
        prices_frame.pack(fill='both', expand=True, padx=10, pady=10)
        
        self.prices_tree = ttk.Treeview(prices_frame, columns=('bid', 'ask', 'last', 'change'), 
                                       height=10)
        self.prices_tree.heading('#0', text='Symbol')
        self.prices_tree.heading('bid', text='Bid')
        self.prices_tree.heading('ask', text='Ask')
        self.prices_tree.heading('last', text='Last')
        self.prices_tree.heading('change', text='24h Change')
        
        self.prices_tree.column('#0', width=100)
        self.prices_tree.column('bid', width=100, anchor='e')
        self.prices_tree.column('ask', width=100, anchor='e')
        self.prices_tree.column('last', width=100, anchor='e')
        self.prices_tree.column('change', width=100, anchor='e')
        
        self.prices_tree.pack(fill='both', expand=True)

    def create_orders_tab(self):
        # Controls
        control_frame = ttk.Frame(self.orders_frame)
        control_frame.pack(fill='x', padx=10, pady=10)
        
        ttk.Button(control_frame, text="Refresh", command=self.refresh_orders).pack(side='left', padx=5)
        ttk.Button(control_frame, text="Cancel Selected", command=self.cancel_selected_order).pack(side='left', padx=5)
        
        # Orders list
        orders_frame = ttk.LabelFrame(self.orders_frame, text="Orders", padding=10)
        orders_frame.pack(fill='both', expand=True, padx=10, pady=10)
        
        self.orders_tree = ttk.Treeview(orders_frame, columns=('symbol', 'side', 'type', 'quantity', 
                                                               'price', 'status', 'time'), height=15)
        self.orders_tree.heading('#0', text='Order ID')
        self.orders_tree.heading('symbol', text='Symbol')
        self.orders_tree.heading('side', text='Side')
        self.orders_tree.heading('type', text='Type')
        self.orders_tree.heading('quantity', text='Quantity')
        self.orders_tree.heading('price', text='Price')
        self.orders_tree.heading('status', text='Status')
        self.orders_tree.heading('time', text='Time')
        
        self.orders_tree.column('#0', width=150)
        self.orders_tree.column('symbol', width=80)
        self.orders_tree.column('side', width=60)
        self.orders_tree.column('type', width=80)
        self.orders_tree.column('quantity', width=100)
        self.orders_tree.column('price', width=100)
        self.orders_tree.column('status', width=80)
        self.orders_tree.column('time', width=150)
        
        scrollbar = ttk.Scrollbar(orders_frame, orient='vertical', command=self.orders_tree.yview)
        self.orders_tree.configure(yscrollcommand=scrollbar.set)
        
        self.orders_tree.pack(side='left', fill='both', expand=True)
        scrollbar.pack(side='right', fill='y')

    def create_balance_tab(self):
        # Controls
        control_frame = ttk.Frame(self.balance_frame)
        control_frame.pack(fill='x', padx=10, pady=10)
        
        ttk.Label(control_frame, text="Market:").pack(side='left', padx=5)
        self.balance_market_var = tk.StringVar(value="spot")
        market_combo = ttk.Combobox(control_frame, textvariable=self.balance_market_var, 
                                   values=["spot", "futures"], state='readonly', width=10)
        market_combo.pack(side='left', padx=5)
        
        ttk.Button(control_frame, text="Refresh", command=self.refresh_balance).pack(side='left', padx=5)
        
        # Balance display
        balance_frame = ttk.LabelFrame(self.balance_frame, text="Balance", padding=10)
        balance_frame.pack(fill='both', expand=True, padx=10, pady=10)
        
        self.balance_tree = ttk.Treeview(balance_frame, columns=('free', 'locked', 'total'), height=15)
        self.balance_tree.heading('#0', text='Asset')
        self.balance_tree.heading('free', text='Free')
        self.balance_tree.heading('locked', text='Locked')
        self.balance_tree.heading('total', text='Total')
        
        self.balance_tree.column('#0', width=100)
        self.balance_tree.column('free', width=150)
        self.balance_tree.column('locked', width=150)
        self.balance_tree.column('total', width=150)
        
        self.balance_tree.pack(fill='both', expand=True)

    def create_market_tab(self):
        # Market data display
        market_frame = ttk.LabelFrame(self.market_frame, text="Market Data", padding=10)
        market_frame.pack(fill='both', expand=True, padx=10, pady=10)
        
        self.market_tree = ttk.Treeview(market_frame, columns=('bid', 'ask', 'last', 'volume', 
                                                               'high', 'low', 'change'), height=20)
        self.market_tree.heading('#0', text='Symbol')
        self.market_tree.heading('bid', text='Bid')
        self.market_tree.heading('ask', text='Ask')
        self.market_tree.heading('last', text='Last')
        self.market_tree.heading('volume', text='24h Volume')
        self.market_tree.heading('high', text='24h High')
        self.market_tree.heading('low', text='24h Low')
        self.market_tree.heading('change', text='24h Change %')
        
        self.market_tree.column('#0', width=100)
        self.market_tree.column('bid', width=100, anchor='e')
        self.market_tree.column('ask', width=100, anchor='e')
        self.market_tree.column('last', width=100, anchor='e')
        self.market_tree.column('volume', width=120, anchor='e')
        self.market_tree.column('high', width=100, anchor='e')
        self.market_tree.column('low', width=100, anchor='e')
        self.market_tree.column('change', width=100, anchor='e')
        
        self.market_tree.pack(fill='both', expand=True)

    def create_log_tab(self):
        # Log display
        self.log_text = scrolledtext.ScrolledText(self.log_frame, wrap='word', height=30)
        self.log_text.pack(fill='both', expand=True, padx=10, pady=10)
        
        # Control buttons
        control_frame = ttk.Frame(self.log_frame)
        control_frame.pack(fill='x', padx=10, pady=5)
        
        ttk.Button(control_frame, text="Clear Log", command=self.clear_log).pack(side='right', padx=5)

    def toggle_price_field(self):
        """Enable/disable price field based on order type"""
        if self.order_type_var.get() == "MARKET":
            self.price_entry.configure(state='disabled')
        else:
            self.price_entry.configure(state='normal')

    def place_order(self):
        """Place an order"""
        try:
            # Get form values
            order_data = {
                'symbol': self.symbol_var.get(),
                'side': self.side_var.get(),
                'order_type': self.order_type_var.get(),
                'quantity': float(self.quantity_var.get()),
                'market': self.market_var.get()
            }
            
            if self.order_type_var.get() == "LIMIT":
                order_data['price'] = float(self.price_var.get())
            
            # Place order
            result = self.client.place_order(**order_data)
            
            # Log success
            self.log(f"Order placed successfully: {result['order_id']}")
            messagebox.showinfo("Success", f"Order placed successfully!\nOrder ID: {result['order_id']}")
            
            # Refresh orders
            self.refresh_orders()
            
        except ValueError as e:
            messagebox.showerror("Error", "Invalid input values. Please check quantity and price.")
        except Exception as e:
            self.log(f"Error placing order: {e}")
            messagebox.showerror("Error", str(e))

    def clear_form(self):
        """Clear order form"""
        self.symbol_var.set("BTCUSDT")
        self.side_var.set("BUY")
        self.order_type_var.set("LIMIT")
        self.quantity_var.set("0.001")
        self.price_var.set("115000")
        self.market_var.set("spot")

    def refresh_orders(self):
        """Refresh orders list"""
        try:
            # Clear existing items
            for item in self.orders_tree.get_children():
                self.orders_tree.delete(item)
            
            # Get orders
            result = self.client.list_orders()
            
            # Add orders to tree
            for order in result.get('orders', []):
                self.orders_tree.insert('', 'end', text=order.get('order_id', ''),
                                      values=(
                                          order.get('symbol', ''),
                                          order.get('side', ''),
                                          order.get('order_type', ''),
                                          order.get('quantity', ''),
                                          order.get('price', ''),
                                          order.get('status', ''),
                                          order.get('created_at', '')
                                      ))
            
            self.log(f"Refreshed orders list: {len(result.get('orders', []))} orders")
            
        except Exception as e:
            self.log(f"Error refreshing orders: {e}")
            messagebox.showerror("Error", f"Failed to refresh orders: {e}")

    def cancel_selected_order(self):
        """Cancel selected order"""
        selection = self.orders_tree.selection()
        if not selection:
            messagebox.showwarning("Warning", "Please select an order to cancel")
            return
        
        order_id = self.orders_tree.item(selection[0])['text']
        
        if messagebox.askyesno("Confirm", f"Cancel order {order_id}?"):
            try:
                result = self.client.cancel_order(order_id)
                self.log(f"Order cancelled: {order_id}")
                messagebox.showinfo("Success", f"Order {order_id} cancelled successfully")
                self.refresh_orders()
            except Exception as e:
                self.log(f"Error cancelling order: {e}")
                messagebox.showerror("Error", str(e))

    def refresh_balance(self):
        """Refresh balance display"""
        try:
            # Clear existing items
            for item in self.balance_tree.get_children():
                self.balance_tree.delete(item)
            
            # Get balance
            result = self.client.get_balance(market=self.balance_market_var.get())
            
            # Add balances to tree
            for balance in result.get('balances', []):
                if balance['total'] > 0:
                    self.balance_tree.insert('', 'end', text=balance['asset'],
                                           values=(
                                               f"{balance['free']:.8f}",
                                               f"{balance['locked']:.8f}",
                                               f"{balance['total']:.8f}"
                                           ))
            
            self.log(f"Refreshed balance: {self.balance_market_var.get()} market")
            
        except Exception as e:
            self.log(f"Error refreshing balance: {e}")
            messagebox.showerror("Error", f"Failed to refresh balance: {e}")

    def update_prices_loop(self):
        """Background thread to update prices"""
        while True:
            try:
                # Get prices
                result = self.client.get_prices()
                self.message_queue.put(('prices', result))
                
            except Exception as e:
                self.message_queue.put(('error', f"Error updating prices: {e}"))
            
            time.sleep(1)  # Update every 1 second

    def process_queue(self):
        """Process messages from background threads"""
        try:
            while True:
                msg_type, data = self.message_queue.get_nowait()
                
                if msg_type == 'prices':
                    self.update_price_displays(data)
                elif msg_type == 'error':
                    self.log(data)
                    
        except queue.Empty:
            pass
        
        # Schedule next check
        self.root.after(100, self.process_queue)

    def update_price_displays(self, price_data):
        """Update price displays"""
        # Clear existing items
        for item in self.prices_tree.get_children():
            self.prices_tree.delete(item)
        
        for item in self.market_tree.get_children():
            self.market_tree.delete(item)
        
        # Sort prices by symbol to maintain consistent order
        prices = sorted(price_data.get('prices', []), key=lambda x: x['symbol'])
        
        # Add prices
        for price in prices:
            # Trading tab
            self.prices_tree.insert('', 'end', text=price['symbol'],
                                  values=(
                                      f"{price['bid_price']:.2f}",
                                      f"{price['ask_price']:.2f}",
                                      f"{price['last_price']:.2f}",
                                      "0.00%"  # Mock change
                                  ))
            
            # Market data tab
            self.market_tree.insert('', 'end', text=price['symbol'],
                                  values=(
                                      f"{price['bid_price']:.2f}",
                                      f"{price['ask_price']:.2f}",
                                      f"{price['last_price']:.2f}",
                                      "0",      # Mock volume
                                      f"{price['last_price'] * 1.02:.2f}",  # Mock high
                                      f"{price['last_price'] * 0.98:.2f}",  # Mock low
                                      "0.00%"   # Mock change
                                  ))

    def log(self, message):
        """Add message to log"""
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        self.log_text.insert('end', f"[{timestamp}] {message}\n")
        self.log_text.see('end')

    def clear_log(self):
        """Clear log display"""
        self.log_text.delete('1.0', 'end')

def main():
    root = tk.Tk()
    
    # Set theme
    style = ttk.Style()
    style.theme_use('clam')
    
    # Configure accent color
    style.configure('Accent.TButton', background='#0078D4', foreground='white')
    style.map('Accent.TButton', background=[('active', '#106EBE')])
    
    app = OMSClientGUI(root)
    root.mainloop()

if __name__ == "__main__":
    main()