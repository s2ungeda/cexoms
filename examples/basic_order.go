package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/your-org/mExOms/pkg/types"
	pb "github.com/your-org/mExOms/pkg/proto/oms/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// gRPC 연결 설정
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// 클라이언트 생성
	client := pb.NewOrderServiceClient(conn)
	ctx := context.Background()

	// 예제 1: 시장가 매수 주문
	fmt.Println("=== Example 1: Market Buy Order ===")
	marketBuyOrder := &pb.CreateOrderRequest{
		Exchange: "binance",
		Market:   "spot",
		Symbol:   "BTC/USDT",
		Side:     pb.OrderSide_BUY,
		Type:     pb.OrderType_MARKET,
		Quantity: 0.001,
	}

	resp1, err := client.CreateOrder(ctx, marketBuyOrder)
	if err != nil {
		log.Printf("Market buy order failed: %v", err)
	} else {
		fmt.Printf("Market buy order placed successfully!\n")
		fmt.Printf("Order ID: %s\n", resp1.Order.OrderId)
		fmt.Printf("Status: %s\n", resp1.Order.Status)
		fmt.Printf("Filled Quantity: %f\n", resp1.Order.FilledQuantity)
		fmt.Printf("Average Price: %f\n\n", resp1.Order.AveragePrice)
	}

	time.Sleep(2 * time.Second)

	// 예제 2: 지정가 매도 주문
	fmt.Println("=== Example 2: Limit Sell Order ===")
	
	// 먼저 현재 가격 조회
	ticker, err := client.GetTicker(ctx, &pb.GetTickerRequest{
		Exchange: "binance",
		Market:   "spot",
		Symbol:   "ETH/USDT",
	})
	if err != nil {
		log.Printf("Failed to get ticker: %v", err)
		return
	}
	
	// 현재 가격보다 1% 높은 가격으로 매도 주문
	sellPrice := ticker.Ticker.LastPrice * 1.01
	
	limitSellOrder := &pb.CreateOrderRequest{
		Exchange: "binance",
		Market:   "spot",
		Symbol:   "ETH/USDT",
		Side:     pb.OrderSide_SELL,
		Type:     pb.OrderType_LIMIT,
		Price:    sellPrice,
		Quantity: 0.01,
		TimeInForce: pb.TimeInForce_GTC, // Good Till Cancelled
	}

	resp2, err := client.CreateOrder(ctx, limitSellOrder)
	if err != nil {
		log.Printf("Limit sell order failed: %v", err)
	} else {
		fmt.Printf("Limit sell order placed successfully!\n")
		fmt.Printf("Order ID: %s\n", resp2.Order.OrderId)
		fmt.Printf("Status: %s\n", resp2.Order.Status)
		fmt.Printf("Price: %f\n", resp2.Order.Price)
		fmt.Printf("Quantity: %f\n\n", resp2.Order.Quantity)
	}

	time.Sleep(2 * time.Second)

	// 예제 3: 주문 상태 조회
	fmt.Println("=== Example 3: Check Order Status ===")
	if resp2 != nil && resp2.Order != nil {
		orderStatus, err := client.GetOrder(ctx, &pb.GetOrderRequest{
			Exchange: "binance",
			Market:   "spot",
			OrderId:  resp2.Order.OrderId,
		})
		if err != nil {
			log.Printf("Failed to get order status: %v", err)
		} else {
			fmt.Printf("Order Status Update:\n")
			fmt.Printf("Order ID: %s\n", orderStatus.Order.OrderId)
			fmt.Printf("Status: %s\n", orderStatus.Order.Status)
			fmt.Printf("Filled: %f/%f\n\n", orderStatus.Order.FilledQuantity, orderStatus.Order.Quantity)
		}
	}

	// 예제 4: 주문 취소
	fmt.Println("=== Example 4: Cancel Order ===")
	if resp2 != nil && resp2.Order != nil && resp2.Order.Status == pb.OrderStatus_OPEN {
		cancelResp, err := client.CancelOrder(ctx, &pb.CancelOrderRequest{
			Exchange: "binance",
			Market:   "spot",
			OrderId:  resp2.Order.OrderId,
		})
		if err != nil {
			log.Printf("Failed to cancel order: %v", err)
		} else {
			fmt.Printf("Order cancelled successfully!\n")
			fmt.Printf("Order ID: %s\n", cancelResp.Order.OrderId)
			fmt.Printf("Final Status: %s\n\n", cancelResp.Order.Status)
		}
	}

	// 예제 5: 활성 주문 목록 조회
	fmt.Println("=== Example 5: List Active Orders ===")
	activeOrders, err := client.ListOrders(ctx, &pb.ListOrdersRequest{
		Exchange: "binance",
		Market:   "spot",
		Status:   pb.OrderStatus_OPEN,
	})
	if err != nil {
		log.Printf("Failed to list active orders: %v", err)
	} else {
		fmt.Printf("Active Orders: %d\n", len(activeOrders.Orders))
		for _, order := range activeOrders.Orders {
			fmt.Printf("- %s: %s %f %s @ %f\n", 
				order.OrderId, 
				order.Side, 
				order.Quantity, 
				order.Symbol, 
				order.Price)
		}
	}

	// 예제 6: 계좌 잔고 조회
	fmt.Println("\n=== Example 6: Account Balance ===")
	balance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{
		Exchange: "binance",
		Market:   "spot",
	})
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
	} else {
		fmt.Println("Account Balances:")
		totalValueUSDT := 0.0
		for _, asset := range balance.Assets {
			if asset.Free > 0 || asset.Locked > 0 {
				fmt.Printf("- %s: Free=%f, Locked=%f\n", asset.Symbol, asset.Free, asset.Locked)
				if asset.Symbol == "USDT" {
					totalValueUSDT += asset.Free + asset.Locked
				}
			}
		}
		fmt.Printf("Total Portfolio Value: ~$%.2f\n", totalValueUSDT)
	}
}

// Helper functions for more complex scenarios

// PlaceStopLossOrder places a stop-loss order
func PlaceStopLossOrder(client pb.OrderServiceClient, symbol string, quantity, stopPrice float64) error {
	ctx := context.Background()
	
	stopOrder := &pb.CreateOrderRequest{
		Exchange:  "binance",
		Market:    "spot",
		Symbol:    symbol,
		Side:      pb.OrderSide_SELL,
		Type:      pb.OrderType_STOP_LOSS,
		StopPrice: stopPrice,
		Quantity:  quantity,
	}
	
	resp, err := client.CreateOrder(ctx, stopOrder)
	if err != nil {
		return fmt.Errorf("failed to place stop-loss order: %w", err)
	}
	
	fmt.Printf("Stop-loss order placed: %s\n", resp.Order.OrderId)
	return nil
}

// PlaceOCOOrder places a One-Cancels-Other order (take profit + stop loss)
func PlaceOCOOrder(client pb.OrderServiceClient, symbol string, quantity, limitPrice, stopPrice float64) error {
	ctx := context.Background()
	
	ocoOrder := &pb.CreateOrderRequest{
		Exchange:     "binance",
		Market:       "spot",
		Symbol:       symbol,
		Side:         pb.OrderSide_SELL,
		Type:         pb.OrderType_LIMIT,
		Price:        limitPrice,  // Take profit price
		StopPrice:    stopPrice,   // Stop loss price
		Quantity:     quantity,
		TimeInForce:  pb.TimeInForce_GTC,
		OrderListId:  fmt.Sprintf("OCO-%d", time.Now().Unix()),
	}
	
	resp, err := client.CreateOrder(ctx, ocoOrder)
	if err != nil {
		return fmt.Errorf("failed to place OCO order: %w", err)
	}
	
	fmt.Printf("OCO order placed: %s\n", resp.Order.OrderId)
	return nil
}