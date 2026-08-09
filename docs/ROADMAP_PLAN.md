# Kanvas 엔터프라이즈 중장기 기술 로드맵 (Product Roadmap Plan)

- **문서 버전**: v0.1.0 ~ v2.0-VISION  
- **작성일자**: 2026년 8월 9일  
- **문서 분류**: 비즈니스 및 아키텍처 중장기 로드맵 (Strategic Product Roadmap)  

---

## 1. 비전 및 발전 마일스톤 개요

Kanvas 플랫폼은 Confluence 데이터의 안전한 온프레미스 이관 및 100% 데이터 주권을 시작으로, 사내 지식을 자동 수집·요약하는 AI 지식 콕핏(Knowledge Cockpit)으로 발전합니다.

```
========================================================================================
                          [Kanvas 단계별 마일스톤 아키텍처]
========================================================================================
 [Phase 1: v0.1.0] (완료) ➔ Confluence Migration Machine, Space/Page ACL, 8 MCP Tools
 [Phase 2: v0.5.0] (진행) ➔ Full-Text Vector Hybrid Search & Multi-Confluence Importer
 [Phase 3: v1.0.0] (2026 Q4) ➔ AI Auto Wiki Draft & Real-Time Knowledge Graph (MCP 2.0)
 [Phase 4: v2.0.0] (2027)    ➔ Autonomous Enterprise Knowledge Copilot & Smart Executive BI
========================================================================================
```

---

## 2. Phase별 세부 기술 명세

### 2.1 Phase 1: v0.1.0 Confluence 이관 및 에어갭 위키 구축 (완료)
- **Confluence Migration Machine**: MySQL Read-Only Discovery, Core/AO 테이블 분류 및 Cutover Gate.
- **Wiki Core & ACL**: Space/Page 구조화 Editor JSON, 불변 Version, User/Group ACL 엔진.
- **Keycloak OIDC & Emergency Bootstrap**: 에어갭 오프라인 단일 컨테이너 및 OIDC PKCE 연동.
- **8 ACL-aware MCP Tools**: AI 에이전트 전용 Streamable HTTP MCP 서버 탑재.

### 2.2 Phase 2: v0.5.0 하이브리드 검색 & 실시간 동기화 (2026 Q3)
- **하이브리드 지식 검색**: 키워드 검색(BM25)과 시맨틱 임베딩 벡터 검색의 조화.
- **다중 Confluence 인스턴스 마이그레이션**: 사업부별 파편화된 Confluence 동시 통합 지원.

### 2.3 Phase 3: v1.0.0 AI 자율 문서 생성 (2026 Q4)
- **AI Auto Wiki Draft (MCP 2.0)**: 사내 회의록, 개발 커밋, 업무 이력을 AI가 자동 파싱하여 위키 페이지 초안 자동 렌더링.

---

## 3. 리소스 및 품질 관리 전략

- **100% 테스트 자동화**: Go 백엔드 유닛 테스트 및 React UI 빌드 검증 자동화.
- **무중단 마이그레이션**: PostgreSQL Migration 자동 스키마 적용 및 백워드 호환성 보장.
