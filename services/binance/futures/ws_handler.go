package futures

import (
	"context"
	"fmt"
	"strings"
	"time"
	
	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/types"
)

// SubscribeKline subscribes to kline/candlestick data
func (bf *BinanceFutures) SubscribeKline(symbol string, interval string) error {
	wsHandler := func(event *futures.WsKlineEvent) {
		kline := &types.FuturesKline{
			Symbol:       event.Symbol,
			Interval:     interval,
			OpenTime:     event.Kline.StartTime,
			CloseTime:    event.Kline.EndTime,
			Open:         parseDecimal(event.Kline.Open),
			High:         parseDecimal(event.Kline.High),
			Low:          parseDecimal(event.Kline.Low),
			Close:        parseDecimal(event.Kline.Close),
			Volume:       parseDecimal(event.Kline.Volume),
			QuoteVolume:  parseDecimal(event.Kline.QuoteVolume),
			Count:        0, // Not available in WsKlineEvent
		}
		
		// Cache latest kline
		cacheKey := fmt.Sprintf("futures:kline:%s:%s", symbol, interval)
		bf.cache.Set(cacheKey, kline, time.Minute)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures Kline WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsKlineServe(symbol, interval, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient[fmt.Sprintf("kline:%s:%s", symbol, interval)] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, fmt.Sprintf("kline:%s:%s", symbol, interval))
	}()
	
	return nil
}

// SubscribeTicker subscribes to 24hr ticker data
func (bf *BinanceFutures) SubscribeTicker(symbol string) error {
	wsHandler := func(event *futures.WsBookTickerEvent) {
		ticker := &types.Ticker{
			Symbol:       event.Symbol,
			BidPrice:     event.BestBidPrice,
			BidQty:       event.BestBidQty,
			AskPrice:     event.BestAskPrice,
			AskQty:       event.BestAskQty,
		}
		
		// Cache ticker
		cacheKey := fmt.Sprintf("futures:ticker:%s", symbol)
		bf.cache.Set(cacheKey, ticker, 5*time.Second)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures Ticker WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsBookTickerServe(symbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient[fmt.Sprintf("ticker:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, fmt.Sprintf("ticker:%s", symbol))
	}()
	
	return nil
}

// SubscribeOrderBook subscribes to order book updates
func (bf *BinanceFutures) SubscribeOrderBook(symbol string, levels int) error {
	wsHandler := func(event *futures.WsDepthEvent) {
		orderBook := &types.FuturesDepth{
			Symbol:       event.Symbol,
			LastUpdateID: event.LastUpdateID,
			Bids:         make([]types.PriceLevel, 0, len(event.Bids)),
			Asks:         make([]types.PriceLevel, 0, len(event.Asks)),
			Timestamp:    parseTimestamp(event.Time),
		}
		
		for _, bid := range event.Bids {
			orderBook.Bids = append(orderBook.Bids, types.PriceLevel{
				Price:    parseDecimal(bid.Price),
				Quantity: parseDecimal(bid.Quantity),
			})
		}
		
		for _, ask := range event.Asks {
			orderBook.Asks = append(orderBook.Asks, types.PriceLevel{
				Price:    parseDecimal(ask.Price),
				Quantity: parseDecimal(ask.Quantity),
			})
		}
		
		// Cache order book
		cacheKey := fmt.Sprintf("futures:orderbook:%s", symbol)
		bf.cache.Set(cacheKey, orderBook, 2*time.Second)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures OrderBook WebSocket error: %v\n", err)
	}
	
	// Convert symbol to lowercase for WebSocket
	wsSymbol := strings.ToLower(symbol)
	doneC, _, err := futures.WsPartialDepthServe(wsSymbol, levels, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient[fmt.Sprintf("orderbook:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, fmt.Sprintf("orderbook:%s", symbol))
	}()
	
	return nil
}

// SubscribeTrades subscribes to trade updates
func (bf *BinanceFutures) SubscribeTrades(symbol string) error {
	wsHandler := func(event *futures.WsAggTradeEvent) {
		trade := &types.FuturesTrade{
			ID:           event.AggregateTradeID,
			Symbol:       event.Symbol,
			Price:        parseDecimal(event.Price),
			Quantity:     parseDecimal(event.Quantity),
			Time:         parseTimestamp(event.Time),
			IsBuyerMaker: event.Maker,
		}
		
		// TODO: Publish to NATS when natsClient is implemented
		_ = trade
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures Trades WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsAggTradeServe(symbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient[fmt.Sprintf("trades:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, fmt.Sprintf("trades:%s", symbol))
	}()
	
	return nil
}

// SubscribeMarkPrice subscribes to mark price updates
func (bf *BinanceFutures) SubscribeMarkPrice(symbol string) error {
	wsHandler := func(event *futures.WsMarkPriceEvent) {
		markPrice := parseDecimal(event.MarkPrice)
		fundingRate := parseDecimal(event.FundingRate)
		
		// Cache mark price
		cacheKey := fmt.Sprintf("futures:markprice:%s", symbol)
		bf.cache.Set(cacheKey, markPrice, 5*time.Second)
		
		// Cache funding rate
		fundingKey := fmt.Sprintf("futures:funding:%s", symbol)
		bf.cache.Set(fundingKey, fundingRate, 5*time.Second)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures MarkPrice WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsMarkPriceServe(symbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient[fmt.Sprintf("markprice:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, fmt.Sprintf("markprice:%s", symbol))
	}()
	
	return nil
}

// SubscribeAllMarkPrices subscribes to all mark price updates
func (bf *BinanceFutures) SubscribeAllMarkPrices() error {
	wsHandler := func(events futures.WsAllMarkPriceEvent) {
		for _, event := range events {
			markPrice := parseDecimal(event.MarkPrice)
			
			// Cache mark price for each symbol
			cacheKey := fmt.Sprintf("futures:markprice:%s", event.Symbol)
			bf.cache.Set(cacheKey, markPrice, 5*time.Second)
		}
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures AllMarkPrices WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsAllMarkPriceServe(wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient["allmarkprices"] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, "allmarkprices")
	}()
	
	return nil
}

// SubscribeLiquidation subscribes to liquidation order updates
func (bf *BinanceFutures) SubscribeLiquidation(symbol string) error {
	wsHandler := func(event *futures.WsLiquidationOrderEvent) {
		// Store the entire liquidation event
		liquidation := event.LiquidationOrder
		
		// Cache liquidation event
		cacheKey := fmt.Sprintf("futures:liquidation:%s", symbol)
		bf.cache.Set(cacheKey, liquidation, 30*time.Second)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures Liquidation WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsLiquidationOrderServe(symbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient[fmt.Sprintf("liquidation:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bf.wsClient, fmt.Sprintf("liquidation:%s", symbol))
	}()
	
	return nil
}

// SubscribeUserData subscribes to user data stream (orders, positions, account)
func (bf *BinanceFutures) SubscribeUserData() error {
	listenKey, err := bf.client.NewStartUserStreamService().Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get listen key: %w", err)
	}
	
	wsHandler := func(event *futures.WsUserDataEvent) {
		switch event.Event {
		case "ORDER_TRADE_UPDATE":
			// Handle order update
			bf.handleOrderUpdate(event)
			
		case "ACCOUNT_UPDATE":
			// Handle account update
			bf.handleAccountUpdate(event)
			
		case "MARGIN_CALL":
			// Handle margin call
			bf.handleMarginCall(event)
		}
	}
	
	errHandler := func(err error) {
		fmt.Printf("Futures UserData WebSocket error: %v\n", err)
	}
	
	doneC, _, err := futures.WsUserDataServe(listenKey, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bf.wsClient["userdata"] = doneC
	
	// Keep listen key alive
	go bf.keepAliveListenKey(listenKey, doneC)
	
	return nil
}

// keepAliveListenKey keeps the user data stream alive
func (bf *BinanceFutures) keepAliveListenKey(listenKey string, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			err := bf.client.NewKeepaliveUserStreamService().
				ListenKey(listenKey).
				Do(context.Background())
			if err != nil {
				fmt.Printf("Failed to keepalive listen key: %v\n", err)
			}
		}
	}
}

// handleOrderUpdate handles order update events
func (bf *BinanceFutures) handleOrderUpdate(event *futures.WsUserDataEvent) {
	// Extract order data from event and update cache
	// The exact structure depends on the event format
	fmt.Printf("Order update received: %+v\n", event)
}

// handleAccountUpdate handles account update events
func (bf *BinanceFutures) handleAccountUpdate(event *futures.WsUserDataEvent) {
	// Extract account data from event and update cache
	// Invalidate account cache to force refresh
	bf.cache.Delete("futures_account")
	fmt.Printf("Account update received: %+v\n", event)
}

// handleMarginCall handles margin call events
func (bf *BinanceFutures) handleMarginCall(event *futures.WsUserDataEvent) {
	// Handle margin call - this is critical
	fmt.Printf("MARGIN CALL received: %+v\n", event)
	// TODO: Send urgent notification
}

// Helper function to parse timestamp
func parseTimestamp(ts int64) time.Time {
	return time.Unix(ts/1000, (ts%1000)*1000000)
}