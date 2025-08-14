package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	// NATS 연결
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatal("NATS 연결 실패:", err)
	}
	defer nc.Close()

	fmt.Println("✓ NATS 연결 성공")

	// JetStream 컨텍스트 생성
	js, err := nc.JetStream()
	if err != nil {
		log.Fatal("JetStream 생성 실패:", err)
	}

	// 스트림 생성
	streamName := "OMS_EVENTS"
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{"orders.>", "market.>", "positions.>"},
		Storage:  nats.FileStorage,
		MaxAge:   24 * time.Hour,
	})
	if err != nil {
		fmt.Printf("스트림 생성 실패 (이미 존재할 수 있음): %v\n", err)
	} else {
		fmt.Println("✓ JetStream 스트림 생성 성공")
	}

	// 테스트 메시지 발행
	subject := "orders.binance.spot.BTCUSDT"
	msg := `{"action":"create","symbol":"BTCUSDT","side":"buy","price":30000,"quantity":0.001}`
	
	_, err = js.Publish(subject, []byte(msg))
	if err != nil {
		log.Fatal("메시지 발행 실패:", err)
	}
	fmt.Printf("✓ 메시지 발행 성공: %s\n", subject)

	// 구독 테스트
	sub, err := js.Subscribe(subject, func(m *nats.Msg) {
		fmt.Printf("✓ 메시지 수신: %s - %s\n", m.Subject, string(m.Data))
		m.Ack()
	})
	if err != nil {
		log.Fatal("구독 실패:", err)
	}
	defer sub.Unsubscribe()

	// 잠시 대기하여 메시지 수신
	time.Sleep(1 * time.Second)

	fmt.Println("\n✓ NATS 테스트 완료!")
}