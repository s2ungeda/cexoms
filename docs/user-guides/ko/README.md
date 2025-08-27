# mExOms 사용자 가이드

다중 거래소 주문 관리 시스템의 다양한 사용자 역할을 위한 포괄적인 가이드입니다.

## 사용자 역할

### 🎯 [트레이더](./trader-guide.md)
- **대상**: 수동 및 자동 거래를 위해 mExOms를 사용하는 활성 트레이더
- **중점사항**: 주문 실행, 포지션 모니터링, 위험 관리
- **도구**: 웹 인터페이스, 거래 봇, 모바일 앱
- **주요 기능**: 다중 거래소 거래, 실시간 데이터, 포트폴리오 분석

### 👨‍💼 [관리자](./admin-guide.md)
- **대상**: mExOms 인프라를 관리하는 시스템 관리자
- **중점사항**: 시스템 배포, 모니터링, 사용자 관리, 보안
- **도구**: 관리자 대시보드, CLI 도구, 모니터링 시스템
- **주요 기능**: 사용자 권한, 시스템 건강성, 성능 최적화

### 👨‍💻 [개발자](./developer-guide.md)
- **대상**: mExOms 기반의 애플리케이션을 개발하는 개발자
- **중점사항**: API 통합, SDK 사용, 커스텀 전략
- **도구**: gRPC/REST API, SDK, 개발 프레임워크
- **주요 기능**: API 문서, 코드 예제, 테스트 도구

### 🏢 [기관 사용자](./institutional-guide.md)
- **대상**: 헤지 펀드, 거래 회사, 마켓 메이커
- **중점사항**: 대량 거래, 컴플라이언스, 위험 관리
- **도구**: 기관용 API, 리포팅 도구, 컴플라이언스 대시보드
- **주요 기능**: 다중 계정 관리, 규제 준수, 고급 분석

## 빠른 탐색

| 원하는 작업 | 가이드 | 섹션 |
|-------------|--------|------|
| 즉시 거래 시작 | [트레이더 가이드](./trader-guide.md) | 빠른 시작 |
| 프로덕션에서 mExOms 배포 | [관리자 가이드](./admin-guide.md) | 배포 |
| 거래 봇 개발 | [개발자 가이드](./developer-guide.md) | API 통합 |
| 컴플라이언스 모니터링 설정 | [기관 가이드](./institutional-guide.md) | 컴플라이언스 |
| 시스템 아키텍처 이해 | [관리자 가이드](./admin-guide.md) | 아키텍처 |
| 커스텀 전략 생성 | [개발자 가이드](./developer-guide.md) | 전략 개발 |

## 일반적인 작업

### 시작하기
1. **인증 설정** → [모든 가이드](./trader-guide.md#authentication)
2. **첫 주문** → [트레이더 가이드](./trader-guide.md#placing-orders)
3. **API 액세스** → [개발자 가이드](./developer-guide.md#api-setup)
4. **시스템 설치** → [관리자 가이드](./admin-guide.md#installation)

### 일일 운영
1. **포지션 모니터링** → [트레이더 가이드](./trader-guide.md#position-management)
2. **시스템 건강성 확인** → [관리자 가이드](./admin-guide.md#monitoring)
3. **거래 성과 검토** → [기관 가이드](./institutional-guide.md#analytics)
4. **API 키 관리** → [개발자 가이드](./developer-guide.md#authentication)

### 고급 기능
1. **다중 계정 거래** → [기관 가이드](./institutional-guide.md#multi-account)
2. **커스텀 전략** → [개발자 가이드](./developer-guide.md#strategies)
3. **고가용성 설정** → [관리자 가이드](./admin-guide.md#high-availability)
4. **컴플라이언스 리포팅** → [기관 가이드](./institutional-guide.md#compliance)

## 지원 리소스

### 문서
- [API 참조](../api/README.md)
- [아키텍처 개요](../architecture/README.md)
- [보안 가이드](../security/README.md)
- [성능 튜닝](../performance/README.md)

### 커뮤니티
- **디스코드**: [mExOms 커뮤니티](https://discord.gg/mexoms)
- **포럼**: [community.mexoms.com](https://community.mexoms.com)
- **GitHub**: [github.com/mexoms/mexoms](https://github.com/mexoms/mexoms)

### 지원
- **이메일**: support@mexoms.com
- **문서 이슈**: docs@mexoms.com
- **보안 이슈**: security@mexoms.com
- **응급상황**: +1-555-MEXOMS

## 피드백

가이드 개선에 도움을 주세요:
- [문서 이슈 신고](https://github.com/mexoms/docs/issues)
- [개선사항 제안](https://github.com/mexoms/docs/discussions)
- [예제 기여](https://github.com/mexoms/docs/pulls)

---

*최종 업데이트: 2025-01-27*