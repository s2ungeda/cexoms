package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	// Redis 연결
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 비밀번호 없음
		DB:       0,  // 기본 DB 사용
	})

	// 연결 테스트
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatal("Redis 연결 실패:", err)
	}
	fmt.Printf("✓ Redis 연결 성공: %s\n", pong)

	// 테스트 데이터 설정
	key := "oms:test:connection"
	value := "Connected at " + time.Now().Format(time.RFC3339)
	
	err = rdb.Set(ctx, key, value, 10*time.Second).Err()
	if err != nil {
		log.Fatal("데이터 설정 실패:", err)
	}
	fmt.Printf("✓ 데이터 설정 성공: %s\n", key)

	// 데이터 읽기
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		log.Fatal("데이터 읽기 실패:", err)
	}
	fmt.Printf("✓ 데이터 읽기 성공: %s\n", val)

	// Hash 테스트 (주문 데이터 저장 예시)
	orderKey := "oms:order:TEST123"
	orderData := map[string]interface{}{
		"symbol":   "BTCUSDT",
		"exchange": "binance",
		"side":     "buy",
		"price":    "30000.00",
		"quantity": "0.001",
		"status":   "new",
		"created":  time.Now().Unix(),
	}

	err = rdb.HMSet(ctx, orderKey, orderData).Err()
	if err != nil {
		log.Fatal("주문 데이터 저장 실패:", err)
	}
	fmt.Printf("✓ 주문 데이터 저장 성공: %s\n", orderKey)

	// Hash 데이터 읽기
	order, err := rdb.HGetAll(ctx, orderKey).Result()
	if err != nil {
		log.Fatal("주문 데이터 읽기 실패:", err)
	}
	fmt.Printf("✓ 주문 데이터 읽기 성공: %+v\n", order)

	// TTL 설정
	rdb.Expire(ctx, orderKey, 60*time.Second)

	// 연결 종료
	rdb.Close()
	fmt.Println("\n✓ Redis 테스트 완료!")
}