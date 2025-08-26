package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/gorilla/websocket"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// FuturesWsOrderManager handles WebSocket order operations for Binance Futures
type FuturesWsOrderManager struct {
	apiKey    string
	apiSecret string
	testnet   bool
	
	// WebSocket connection
	listenKey     string
	conn          *websocket.Conn
	stopCh        chan struct{}
	connected     bool
	mu            sync.RWMutex
	
	// Client for REST fallback and listen key management
	client        *futures.Client
	
	// Callbacks
	orderCallbacks   []types.OrderUpdateCallback
	positionCallbacks []types.PositionUpdateCallback
	accountCallbacks  []types.AccountUpdateCallback
}

// NewFuturesWsOrderManager creates a new WebSocket order manager
func NewFuturesWsOrderManager(apiKey, apiSecret string, testnet bool) *FuturesWsOrderManager {
	if testnet {
		futures.UseTestnet = true
	}
	
	client := futures.NewClient(apiKey, apiSecret)
	
	return &FuturesWsOrderManager{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		testnet:   testnet,
		client:    client,
	}
}

// Initialize starts the WebSocket connection
func (wm *FuturesWsOrderManager) Initialize(ctx context.Context) error {
	// Get listen key
	listenKey, err := wm.client.NewStartUserStreamService().Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to get listen key: %v", err)
	}
	
	wm.mu.Lock()
	wm.listenKey = listenKey
	wm.mu.Unlock()
	
	// Connect to user data stream
	wsURL := fmt.Sprintf("wss://fstream.binance.com/ws/%s", listenKey)
	if wm.testnet {
		wsURL = fmt.Sprintf("wss://stream.binancefuture.com/ws/%s", listenKey)
	}
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to user stream: %v", err)
	}
	
	wm.mu.Lock()
	wm.conn = conn
	wm.stopCh = make(chan struct{})
	wm.connected = true
	wm.mu.Unlock()
	
	// Start reading messages
	go wm.readMessages()
	
	// Start keepalive
	go wm.keepAlive(ctx)
	
	log.Printf("Futures WebSocket order manager connected")
	return nil
}

// IsConnected returns whether the WebSocket is connected
func (wm *FuturesWsOrderManager) IsConnected() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.connected
}

// PlaceOrder places an order via WebSocket (Binance doesn't support this - fallback to REST)
func (wm *FuturesWsOrderManager) PlaceOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	// Binance doesn't support order placement via WebSocket
	// This is here for interface compatibility
	return nil, fmt.Errorf("WebSocket order placement not supported by Binance")
}

// CancelOrder cancels an order via WebSocket (Binance doesn't support this - fallback to REST)
func (wm *FuturesWsOrderManager) CancelOrder(ctx context.Context, symbol, orderID, clientOrderID string) error {
	// Binance doesn't support order cancellation via WebSocket
	// This is here for interface compatibility
	return fmt.Errorf("WebSocket order cancellation not supported by Binance")
}

// RegisterOrderCallback registers a callback for order updates
func (wm *FuturesWsOrderManager) RegisterOrderCallback(callback types.OrderUpdateCallback) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.orderCallbacks = append(wm.orderCallbacks, callback)
}

// RegisterPositionCallback registers a callback for position updates
func (wm *FuturesWsOrderManager) RegisterPositionCallback(callback types.PositionUpdateCallback) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.positionCallbacks = append(wm.positionCallbacks, callback)
}

// RegisterAccountCallback registers a callback for account updates
func (wm *FuturesWsOrderManager) RegisterAccountCallback(callback types.AccountUpdateCallback) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.accountCallbacks = append(wm.accountCallbacks, callback)
}

// readMessages reads messages from the WebSocket
func (wm *FuturesWsOrderManager) readMessages() {
	defer func() {
		wm.mu.Lock()
		wm.connected = false
		if wm.conn != nil {
			wm.conn.Close()
		}
		wm.mu.Unlock()
	}()
	
	for {
		select {
		case <-wm.stopCh:
			return
		default:
			_, message, err := wm.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Futures WebSocket error: %v", err)
				}
				// Try to reconnect
				go wm.reconnect()
				return
			}
			
			// Parse and handle message
			wm.handleMessage(message)
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (wm *FuturesWsOrderManager) handleMessage(message []byte) {
	// Parse message type first
	var baseMsg struct {
		EventType string `json:"e"`
		EventTime int64  `json:"E"`
	}
	
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		log.Printf("Failed to parse WebSocket message: %v", err)
		return
	}
	
	switch baseMsg.EventType {
	case "ORDER_TRADE_UPDATE":
		wm.handleOrderUpdate(message)
	case "ACCOUNT_UPDATE":
		wm.handleAccountUpdate(message)
	case "listenKeyExpired":
		log.Println("Listen key expired, reconnecting...")
		go wm.reconnect()
	}
}

// handleOrderUpdate processes order update events
func (wm *FuturesWsOrderManager) handleOrderUpdate(message []byte) {
	var event struct {
		EventType     string `json:"e"`
		EventTime     int64  `json:"E"`
		TransactTime  int64  `json:"T"`
		Order struct {
			Symbol             string `json:"s"`
			ClientOrderID      string `json:"c"`
			Side               string `json:"S"`
			OrderType          string `json:"o"`
			TimeInForce        string `json:"f"`
			OriginalQuantity   string `json:"q"`
			Price              string `json:"p"`
			AveragePrice       string `json:"ap"`
			StopPrice          string `json:"sp"`
			ExecutionType      string `json:"x"`
			OrderStatus        string `json:"X"`
			OrderID            int64  `json:"i"`
			LastFilledQuantity string `json:"l"`
			FilledAccumulatedQuantity string `json:"z"`
			LastFilledPrice    string `json:"L"`
			CommissionAsset    string `json:"N"`
			Commission         string `json:"n"`
			OrderTradeTime     int64  `json:"T"`
			TradeID            int64  `json:"t"`
			BidsNotional       string `json:"b"`
			AsksNotional       string `json:"a"`
			IsMaker            bool   `json:"m"`
			IsReduceOnly       bool   `json:"R"`
			WorkingType        string `json:"wt"`
			OriginalOrderType  string `json:"ot"`
			PositionSide       string `json:"ps"`
			IsCloseAll         bool   `json:"cp"`
			ActivationPrice    string `json:"AP"`
			CallbackRate       string `json:"cr"`
			RealizedProfit     string `json:"rp"`
		} `json:"o"`
	}
	
	if err := json.Unmarshal(message, &event); err != nil {
		log.Printf("Failed to parse order update: %v", err)
		return
	}
	
	// Convert to OrderUpdate
	update := &types.OrderUpdate{
		Symbol:           event.Order.Symbol,
		ClientOrderID:    event.Order.ClientOrderID,
		Side:             event.Order.Side,
		Type:             event.Order.OrderType,
		TimeInForce:      event.Order.TimeInForce,
		Quantity:         parseDecimalString(event.Order.OriginalQuantity),
		Price:            parseDecimalString(event.Order.Price),
		StopPrice:        parseDecimalString(event.Order.StopPrice),
		Status:           event.Order.OrderStatus,
		OrderID:          fmt.Sprintf("%d", event.Order.OrderID),
		LastFilledQty:    parseDecimalString(event.Order.LastFilledQuantity),
		FilledQty:        parseDecimalString(event.Order.FilledAccumulatedQuantity),
		LastPrice:        parseDecimalString(event.Order.LastFilledPrice),
		FeeAsset:         event.Order.CommissionAsset,
		Fee:              parseDecimalString(event.Order.Commission),
		Time:             event.Order.OrderTradeTime,
		UpdateTime:       event.EventTime,
		ExecutionType:    event.Order.ExecutionType,
		TradeID:          event.Order.TradeID,
		IsMaker:          event.Order.IsMaker,
		ReduceOnly:       event.Order.IsReduceOnly,
		PositionSide:     event.Order.PositionSide,
		RealizedPnL:      parseDecimalString(event.Order.RealizedProfit),
	}
	
	// Call callbacks
	wm.mu.RLock()
	callbacks := wm.orderCallbacks
	wm.mu.RUnlock()
	
	for _, callback := range callbacks {
		go callback(update)
	}
}

// handleAccountUpdate processes account update events
func (wm *FuturesWsOrderManager) handleAccountUpdate(message []byte) {
	var event struct {
		EventType    string `json:"e"`
		EventTime    int64  `json:"E"`
		TransactTime int64  `json:"T"`
		Account struct {
			Balances []struct {
				Asset              string `json:"a"`
				WalletBalance      string `json:"wb"`
				CrossWalletBalance string `json:"cw"`
				BalanceChange      string `json:"bc"`
			} `json:"B"`
			Positions []struct {
				Symbol                string `json:"s"`
				PositionAmount        string `json:"pa"`
				EntryPrice            string `json:"ep"`
				AccumulatedRealized   string `json:"cr"`
				UnrealizedPnL         string `json:"up"`
				MarginType            string `json:"mt"`
				IsolatedWallet        string `json:"iw"`
				PositionSide          string `json:"ps"`
			} `json:"P"`
		} `json:"a"`
		EventReasonType string `json:"m"`
	}
	
	if err := json.Unmarshal(message, &event); err != nil {
		log.Printf("Failed to parse account update: %v", err)
		return
	}
	
	// Handle position updates
	for _, pos := range event.Account.Positions {
		positionQty, _ := strconv.ParseFloat(pos.PositionAmount, 64)
		if positionQty == 0 {
			continue // Skip empty positions
		}
		
		side := types.PositionSideNone
		if positionQty > 0 {
			side = types.PositionSideLong
		} else {
			side = types.PositionSideShort
			positionQty = -positionQty
		}
		
		position := &types.Position{
			Symbol:        pos.Symbol,
			Side:          side,
			PositionSide:  pos.PositionSide,
			Quantity:      decimal.NewFromFloat(positionQty),
			EntryPrice:    parseDecimalString(pos.EntryPrice),
			UnrealizedPnL: parseDecimalString(pos.UnrealizedPnL),
			RealizedPnL:   parseDecimalString(pos.AccumulatedRealized),
			MarginType:    pos.MarginType,
			UpdateTime:    time.Unix(event.EventTime/1000, 0),
		}
		
		// Call position callbacks
		wm.mu.RLock()
		posCallbacks := wm.positionCallbacks
		wm.mu.RUnlock()
		
		for _, callback := range posCallbacks {
			go callback(position)
		}
	}
	
	// Handle account balance updates
	if len(event.Account.Balances) > 0 {
		accountUpdate := &types.AccountUpdate{
			EventTime: event.EventTime,
			Reason:    event.EventReasonType,
			Balances:  make(map[string]types.BalanceUpdate),
		}
		
		for _, bal := range event.Account.Balances {
			walletBalance, _ := strconv.ParseFloat(bal.WalletBalance, 64)
			crossWalletBalance, _ := strconv.ParseFloat(bal.CrossWalletBalance, 64)
			balanceChange, _ := strconv.ParseFloat(bal.BalanceChange, 64)
			
			accountUpdate.Balances[bal.Asset] = types.BalanceUpdate{
				Asset:          bal.Asset,
				Free:           decimal.NewFromFloat(walletBalance),
				Locked:         decimal.Zero, // Not provided in futures
				Total:          decimal.NewFromFloat(crossWalletBalance),
				BalanceChange:  decimal.NewFromFloat(balanceChange),
				UpdateTime:     event.EventTime,
			}
		}
		
		// Call account callbacks
		wm.mu.RLock()
		accCallbacks := wm.accountCallbacks
		wm.mu.RUnlock()
		
		for _, callback := range accCallbacks {
			go callback(accountUpdate)
		}
	}
}

// keepAlive keeps the listen key alive
func (wm *FuturesWsOrderManager) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-wm.stopCh:
			return
		case <-ticker.C:
			wm.mu.RLock()
			listenKey := wm.listenKey
			wm.mu.RUnlock()
			
			if listenKey != "" {
				err := wm.client.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(ctx)
				if err != nil {
					log.Printf("Failed to keepalive futures listen key: %v", err)
					// Try to reconnect
					go wm.reconnect()
				}
			}
		}
	}
}

// reconnect attempts to reconnect the WebSocket
func (wm *FuturesWsOrderManager) reconnect() {
	wm.mu.Lock()
	if wm.conn != nil {
		wm.conn.Close()
	}
	wm.connected = false
	wm.mu.Unlock()
	
	// Wait before reconnecting
	time.Sleep(5 * time.Second)
	
	ctx := context.Background()
	if err := wm.Initialize(ctx); err != nil {
		log.Printf("Failed to reconnect futures WebSocket: %v", err)
		// Try again
		go wm.reconnect()
	}
}

// Close closes the WebSocket connection
func (wm *FuturesWsOrderManager) Close() error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	
	if wm.stopCh != nil {
		close(wm.stopCh)
	}
	
	if wm.conn != nil {
		return wm.conn.Close()
	}
	
	return nil
}

// Helper function
func parseDecimalString(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, _ := decimal.NewFromString(s)
	return d
}