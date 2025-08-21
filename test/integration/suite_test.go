package integration

import (
	"flag"
	"os"
	"testing"
)

var (
	// 통합 테스트 플래그
	runIntegration = flag.Bool("integration", false, "통합 테스트 실행")
	useTestnet     = flag.Bool("testnet", true, "테스트넷 사용")
	verbose        = flag.Bool("v", false, "자세한 출력")
)

func TestMain(m *testing.M) {
	flag.Parse()
	
	// 통합 테스트 환경 설정
	if *runIntegration {
		setupIntegrationTests()
	}
	
	// 테스트 실행
	exitCode := m.Run()
	
	// 정리
	if *runIntegration {
		teardownIntegrationTests()
	}
	
	os.Exit(exitCode)
}

func setupIntegrationTests() {
	// 환경 변수 설정
	if *useTestnet {
		os.Setenv("BINANCE_TESTNET", "true")
		os.Setenv("BYBIT_TESTNET", "true")
		os.Setenv("OKX_TESTNET", "true")
	}
	
	// 로그 레벨 설정
	if *verbose {
		os.Setenv("LOG_LEVEL", "debug")
	}
}

func teardownIntegrationTests() {
	// 필요한 정리 작업
}