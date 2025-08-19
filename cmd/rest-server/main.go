package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/mExOms/internal/marketdata"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type RestServer struct {
	grpcClient OrderServiceClient
	aggregator *marketdata.Aggregator
}

// Placeholder for gRPC client interface
type OrderServiceClient interface{}

type PlaceOrderRequest struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	OrderType string  `json:"order_type"`
	Quantity  float64 `json:"quantity"`
	Price     float64 `json:"price,omitempty"`
	Exchange  string  `json:"exchange,omitempty"`
	Market    string  `json:"market,omitempty"`
	AccountID string  `json:"account_id,omitempty"`
}

type PlaceOrderResponse struct {
	OrderID         string    `json:"order_id"`
	ExchangeOrderID string    `json:"exchange_order_id"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type Balance struct {
	Asset  string  `json:"asset"`
	Free   float64 `json:"free"`
	Locked float64 `json:"locked"`
	Total  float64 `json:"total"`
}

type Position struct {
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Size          float64 `json:"size"`
	EntryPrice    float64 `json:"entry_price"`
	MarkPrice     float64 `json:"mark_price"`
	UnrealizedPnl float64 `json:"unrealized_pnl"`
	PnlPercentage float64 `json:"pnl_percentage"`
	Leverage      int     `json:"leverage"`
	Margin        float64 `json:"margin"`
}

type PriceUpdate struct {
	Exchange     string    `json:"exchange"`
	Symbol       string    `json:"symbol"`
	BidPrice     float64   `json:"bid_price"`
	BidQuantity  float64   `json:"bid_quantity"`
	AskPrice     float64   `json:"ask_price"`
	AskQuantity  float64   `json:"ask_quantity"`
	LastPrice    float64   `json:"last_price"`
	Timestamp    time.Time `json:"timestamp"`
}

func main() {
	// Connect to gRPC server
	grpcAddr := os.Getenv("GRPC_SERVER")
	if grpcAddr == "" {
		grpcAddr = "localhost:50051"
	}

	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	// Connect to NATS for market data
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	aggregator, err := marketdata.NewAggregator(natsURL)
	if err != nil {
		log.Printf("Warning: Failed to create market data aggregator: %v", err)
		log.Println("REST server will run with mock data")
	} else {
		if err := aggregator.Start(); err != nil {
			log.Printf("Warning: Failed to start aggregator: %v", err)
		}
		defer aggregator.Stop()
	}

	// Create REST server
	server := &RestServer{
		// grpcClient: proto.NewOrderServiceClient(conn),
		aggregator: aggregator,
	}

	// Setup routes
	router := mux.NewRouter()
	
	// CORS middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	})

	// API routes
	api := router.PathPrefix("/api/v1").Subrouter()
	
	// Order endpoints
	api.HandleFunc("/orders", server.placeOrder).Methods("POST")
	api.HandleFunc("/orders/{id}", server.getOrder).Methods("GET")
	api.HandleFunc("/orders/{id}", server.cancelOrder).Methods("DELETE")
	api.HandleFunc("/orders", server.listOrders).Methods("GET")
	
	// Account endpoints
	api.HandleFunc("/balance", server.getBalance).Methods("GET")
	api.HandleFunc("/positions", server.getPositions).Methods("GET")
	
	// Market data endpoints
	api.HandleFunc("/prices", server.getPrices).Methods("GET")
	api.HandleFunc("/ticker/{symbol}", server.getTicker).Methods("GET")
	
	// Health check
	api.HandleFunc("/health", server.healthCheck).Methods("GET")

	// Serve static files for web UI
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web")))

	// Start server
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("REST API server starting on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

// Handler implementations
func (s *RestServer) placeOrder(w http.ResponseWriter, r *http.Request) {
	var req PlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Symbol == "" || req.Side == "" || req.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "Missing required fields")
		return
	}

	// Set defaults
	if req.OrderType == "" {
		req.OrderType = "LIMIT"
	}
	if req.Exchange == "" {
		req.Exchange = "binance"
	}
	if req.Market == "" {
		req.Market = "spot"
	}
	if req.AccountID == "" {
		req.AccountID = "main"
	}

	// TODO: Call gRPC service
	// For now, return mock response
	resp := PlaceOrderResponse{
		OrderID:         fmt.Sprintf("ORD-%d", time.Now().Unix()),
		ExchangeOrderID: fmt.Sprintf("EX-%d", time.Now().Unix()),
		Status:          "NEW",
		CreatedAt:       time.Now(),
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *RestServer) getOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderID := vars["id"]

	// TODO: Call gRPC service
	// For now, return mock response
	order := map[string]interface{}{
		"order_id":          orderID,
		"exchange_order_id": "EX-123456",
		"symbol":            "BTCUSDT",
		"side":              "BUY",
		"order_type":        "LIMIT",
		"quantity":          0.001,
		"price":             115000,
		"filled_quantity":   0,
		"status":            "NEW",
		"exchange":          "binance",
		"market":            "spot",
		"account_id":        "main",
		"created_at":        time.Now(),
	}

	writeJSON(w, http.StatusOK, order)
}

func (s *RestServer) cancelOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderID := vars["id"]

	// TODO: Call gRPC service
	// For now, return mock response
	resp := map[string]interface{}{
		"order_id":     orderID,
		"status":       "CANCELLED",
		"cancelled_at": time.Now(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *RestServer) listOrders(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	symbol := r.URL.Query().Get("symbol")
	limit := r.URL.Query().Get("limit")

	_ = status
	_ = symbol
	
	limitInt := 100
	if limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			limitInt = l
		}
	}

	// TODO: Call gRPC service
	// For now, return empty list
	orders := []interface{}{}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"orders": orders,
		"count":  len(orders),
		"limit":  limitInt,
	})
}

func (s *RestServer) getBalance(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	market := r.URL.Query().Get("market")
	accountID := r.URL.Query().Get("account_id")

	if exchange == "" {
		exchange = "binance"
	}
	if market == "" {
		market = "spot"
	}
	if accountID == "" {
		accountID = "main"
	}

	// TODO: Call gRPC service
	// For now, return mock balances
	balances := []Balance{
		{Asset: "USDT", Free: 10000, Locked: 0, Total: 10000},
		{Asset: "BTC", Free: 0.5, Locked: 0, Total: 0.5},
		{Asset: "ETH", Free: 5, Locked: 0, Total: 5},
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exchange":   exchange,
		"market":     market,
		"account_id": accountID,
		"balances":   balances,
	})
}

func (s *RestServer) getPositions(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	accountID := r.URL.Query().Get("account_id")

	if exchange == "" {
		exchange = "binance"
	}
	if accountID == "" {
		accountID = "main"
	}

	// TODO: Call gRPC service
	// For now, return empty positions
	positions := []Position{}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exchange":   exchange,
		"account_id": accountID,
		"positions":  positions,
	})
}

func (s *RestServer) getPrices(w http.ResponseWriter, r *http.Request) {
	symbols := r.URL.Query()["symbol"]
	
	// Use aggregator if available, otherwise fall back to mock data
	if s.aggregator != nil {
		// Get real prices from aggregator
		priceData := s.aggregator.GetPrices(symbols)
		
		// Convert to REST API format
		prices := make([]PriceUpdate, 0, len(priceData))
		for _, pd := range priceData {
			prices = append(prices, PriceUpdate{
				Exchange:     pd.Exchange,
				Symbol:       pd.Symbol,
				BidPrice:     pd.BidPrice,
				BidQuantity:  pd.BidQuantity,
				AskPrice:     pd.AskPrice,
				AskQuantity:  pd.AskQuantity,
				LastPrice:    pd.LastPrice,
				Timestamp:    pd.Timestamp,
			})
		}
		
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"prices": prices,
			"count":  len(prices),
		})
		return
	}
	
	// Fall back to mock data
	if len(symbols) == 0 {
		symbols = []string{"BTCUSDT", "ETHUSDT", "XRPUSDT"}
	}

	prices := []PriceUpdate{}
	for _, symbol := range symbols {
		prices = append(prices, PriceUpdate{
			Exchange:     "binance",
			Symbol:       symbol,
			BidPrice:     115000,
			BidQuantity:  0.5,
			AskPrice:     115010,
			AskQuantity:  0.5,
			LastPrice:    115005,
			Timestamp:    time.Now(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prices": prices,
		"count":  len(prices),
	})
}

func (s *RestServer) getTicker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	symbol := vars["symbol"]

	// TODO: Call gRPC service
	// For now, return mock ticker
	ticker := map[string]interface{}{
		"symbol":       symbol,
		"bid_price":    115000,
		"bid_quantity": 0.5,
		"ask_price":    115010,
		"ask_quantity": 0.5,
		"last_price":   115005,
		"volume_24h":   1234567,
		"high_24h":     116000,
		"low_24h":      114000,
		"change_24h":   0.02,
		"timestamp":    time.Now(),
	}

	writeJSON(w, http.StatusOK, ticker)
}

func (s *RestServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
		"services": map[string]string{
			"grpc": "connected",
		},
	}

	writeJSON(w, http.StatusOK, health)
}

// Helper functions
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}