package binance

// WebSocketStream holds WebSocket stream control channels
type WebSocketStream struct {
	Done chan struct{}
	Stop chan struct{}
}