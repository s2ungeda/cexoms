# mExOms TCP Server

고성능 C++ TCP 서버로 20-100명의 클라이언트에게 실시간 시장 데이터와 주문 처리 서비스를 제공합니다.

## 주요 특징

- **고성능**: < 100μs 레이턴시, 50,000 msg/sec/client
- **확장성**: 20-100 동시 클라이언트 지원
- **실시간**: Lock-free 큐, Zero-copy 최적화
- **안정성**: Epoll 기반 비동기 I/O (Linux)

## 빌드 방법

```bash
# 빌드 스크립트 사용
./build_tcp_server.sh

# 또는 수동 빌드
cd core/build
cmake ..
make tcp-server tcp-client-example
```

## 실행 방법

### 서버 시작
```bash
./bin/tcp-server [port]
# 기본 포트: 9090
```

### 클라이언트 예제
```bash
./bin/tcp-client-example [host] [port]
# 기본: localhost 9090
```

## 테스트

```bash
# 자동 테스트 스크립트
./test_tcp_server.sh
```

## 프로토콜

Protocol Buffers 기반 바이너리 프로토콜을 사용합니다.
자세한 내용은 [TCP_PROTOCOL.md](TCP_PROTOCOL.md) 참조.

## 아키텍처

```
┌─────────────┐     ┌─────────────┐
│ TCP Client  │────▶│ TCP Server  │
└─────────────┘     └──────┬──────┘
                           │
                    ┌──────┴──────┐
                    │   Epoll     │
                    │   Worker     │
                    │   Threads    │
                    └──────┬──────┘
                           │
                ┌──────────┴──────────┐
                │                     │
         ┌──────┴──────┐      ┌──────┴──────┐
         │ Message     │      │ Broadcaster │
         │ Handler     │      │             │
         └─────────────┘      └─────────────┘
```

## 성능 최적화

1. **Lock-free 큐**: 메시지 전달
2. **Zero-copy**: 가능한 곳에서 메모리 복사 최소화
3. **Edge-triggered epoll**: 효율적인 이벤트 처리
4. **메시지 캐싱**: 반복되는 메시지 직렬화 캐싱

## 구현 상태

- ✅ 기본 TCP 서버
- ✅ Linux epoll 지원
- ✅ 클라이언트 세션 관리
- ✅ 메시지 브로드캐스팅
- ✅ Protocol Buffers 통합
- ✅ 인증 및 권한 관리
- ⏳ Windows IOCP (선택사항)
- ⏳ TLS 암호화 (향후)

## 파일 구조

```
core/tcp_server/
├── tcp_server.h          # 서버 메인 클래스
├── tcp_server.cpp        # 서버 구현
├── tcp_server_linux.cpp  # Linux epoll 구현
├── client_session.h      # 클라이언트 세션
├── client_session.cpp    # 세션 구현
├── message_handler.h     # 메시지 처리
├── message_handler.cpp   # 메시지 핸들러 구현
├── broadcaster.h         # 브로드캐스터
├── broadcaster.cpp       # 브로드캐스터 구현
└── tcp_server_utils.cpp  # 유틸리티 함수
```

## 향후 개선사항

1. **보안**: TLS 1.3 지원
2. **모니터링**: Prometheus 메트릭
3. **확장성**: 클러스터링 지원
4. **프로토콜**: gRPC 호환성