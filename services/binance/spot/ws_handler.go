package spot

import (
	"fmt"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

func (bs *BinanceSpot) SubscribeKline(symbol string, interval string) error {
	wsHandler := func(event *binance.WsKlineEvent) {
		kline := &types.Kline{
			Symbol:    event.Symbol,
			Interval:  interval,
			OpenTime:  event.Kline.StartTime,
			CloseTime: event.Kline.EndTime,
			Open:      event.Kline.Open,
			High:      event.Kline.High,
			Low:       event.Kline.Low,
			Close:     event.Kline.Close,
			Volume:    event.Kline.Volume,
			IsFinal:   event.Kline.IsFinal,
		}
		
		// Cache latest kline
		cacheKey := fmt.Sprintf("kline:%s:%s", symbol, interval)
		bs.cache.Set(cacheKey, kline, time.Minute)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Kline WebSocket error: %v\n", err)
	}
	
	doneC, _, err := binance.WsKlineServe(symbol, interval, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	// Store the channel for cleanup
	bs.wsClient[fmt.Sprintf("kline:%s:%s", symbol, interval)] = doneC
	
	// Handle done channel in goroutine
	go func() {
		<-doneC
		delete(bs.wsClient, fmt.Sprintf("kline:%s:%s", symbol, interval))
	}()
	
	return nil
}

func (bs *BinanceSpot) SubscribeTicker(symbol string) error {
	wsHandler := func(event *binance.WsMarketStatEvent) {
		ticker := &types.Ticker{
			Symbol:       event.Symbol,
			Price:        event.LastPrice,
			Volume:       event.BaseVolume,
			QuoteVolume:  event.QuoteVolume,
			BidPrice:     event.BidPrice,
			BidQty:       event.BidQty,
			AskPrice:     event.AskPrice,
			AskQty:       event.AskQty,
			High:         event.HighPrice,
			Low:          event.LowPrice,
			Open:         event.OpenPrice,
			PriceChange:  event.PriceChange,
			PricePercent: event.PriceChangePercent,
		}
		
		// Cache ticker
		cacheKey := fmt.Sprintf("ticker:%s", symbol)
		bs.cache.Set(cacheKey, ticker, 5*time.Second)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("Ticker WebSocket error: %v\n", err)
	}
	
	doneC, _, err := binance.WsMarketStatServe(symbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bs.wsClient[fmt.Sprintf("ticker:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bs.wsClient, fmt.Sprintf("ticker:%s", symbol))
	}()
	
	return nil
}

func (bs *BinanceSpot) SubscribeOrderBook(symbol string, levels int) error {
	wsHandler := func(event *binance.WsDepthEvent) {
		orderBook := &types.OrderBook{
			Symbol:       event.Symbol,
			LastUpdateID: event.LastUpdateID,
			Bids:         make([]types.PriceLevel, 0, len(event.Bids)),
			Asks:         make([]types.PriceLevel, 0, len(event.Asks)),
		}
		
		for _, bid := range event.Bids {
			price, _ := decimal.NewFromString(bid.Price)
			quantity, _ := decimal.NewFromString(bid.Quantity)
			orderBook.Bids = append(orderBook.Bids, types.PriceLevel{
				Price:    price,
				Quantity: quantity,
			})
		}
		
		for _, ask := range event.Asks {
			price, _ := decimal.NewFromString(ask.Price)
			quantity, _ := decimal.NewFromString(ask.Quantity)
			orderBook.Asks = append(orderBook.Asks, types.PriceLevel{
				Price:    price,
				Quantity: quantity,
			})
		}
		
		// Cache order book
		cacheKey := fmt.Sprintf("orderbook:%s", symbol)
		bs.cache.Set(cacheKey, orderBook, 2*time.Second)
		
		// TODO: Publish to NATS when natsClient is implemented
	}
	
	errHandler := func(err error) {
		fmt.Printf("OrderBook WebSocket error: %v\n", err)
	}
	
	// Convert symbol to lowercase for WebSocket
	wsSymbol := strings.ToLower(symbol)
	doneC, _, err := binance.WsDepthServe(wsSymbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bs.wsClient[fmt.Sprintf("orderbook:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bs.wsClient, fmt.Sprintf("orderbook:%s", symbol))
	}()
	
	return nil
}

func (bs *BinanceSpot) SubscribeTrades(symbol string) error {
	wsHandler := func(event *binance.WsTradeEvent) {
		trade := &types.Trade{
			ID:           fmt.Sprintf("%d", event.TradeID),
			Symbol:       event.Symbol,
			Price:        event.Price,
			Quantity:     event.Quantity,
			Time:         event.Time,
			IsBuyerMaker: event.IsBuyerMaker,
		}
		
		// TODO: Publish to NATS when natsClient is implemented
		_ = trade
	}
	
	errHandler := func(err error) {
		fmt.Printf("Trades WebSocket error: %v\n", err)
	}
	
	doneC, _, err := binance.WsTradeServe(symbol, wsHandler, errHandler)
	if err != nil {
		return err
	}
	
	bs.wsClient[fmt.Sprintf("trades:%s", symbol)] = doneC
	
	go func() {
		<-doneC
		delete(bs.wsClient, fmt.Sprintf("trades:%s", symbol))
	}()
	
	return nil
}