# Kanvas 엔터프라이즈 관리자 가이드 (Admin & Operational Guide)

- **문서 버전**: v0.1.0-ENTERPRISE  
- **대상**: 시스템 관리자, Security/DevOps 엔지니어, 데이터 마이그레이션 담당자  
- **문서 개요**: Kanvas 4대 환경변수 부트스트랩, Confluence Discovery & Migration Machine 제어, Keycloak OIDC SSO 및 감사 로그 운영  

---

## 1. 시스템 부트스트랩 (Bootstrap Environment Variables)

Kanvas 프로세스는 오직 **4개의 필수 환경변수**만으로 전체 시스템 부트스트랩을 완수합니다.

```bash
# deploy 환경변수 설정 예시
KANVAS_POSTGRES_DSN=postgres://kanvas:Secr3tPass@10.10.40.5:5432/kanvas?sslmode=disable
KANVAS_CONFLUENCE_DSN=root:ReadOnlyMySQLPass@tcp(10.10.40.8:3306)/confluence
KANVAS_BOOTSTRAP_ADMIN=admin
KANVAS_BOOTSTRAP_ADMIN_PASSWORD=SuperSecretAdminPassword123!
```

> **설정 원칙**:  
> `KANVAS_CONFLUENCE_DSN`은 마이그레이션 탐색 전까지 빈 문자열(`""`)로 설정 가능합니다. 부트스트랩 관리자는 최초 계정이 없을 때만 생성되며, OIDC 설정, 첨부파일 경로, Migration 상태는 모두 관리자 대시보드에서 동적으로 제어됩니다.

---

## 2. Confluence Migration Machine 및 Cutover Gate 운영

Confluence 데이터를 무중단 이관하기 위한 4단계 상태 머신 운영 절차:

1. **DISCOVERY**: Read-Only Confluence MySQL 스키마 탐색 및 연결 검증.
2. **CLASSIFY**: 테이블을 `Core` (문서, 사용자, 스페이스), `AO` (ActiveObjects 플러그인), `Unknown` 세 카테고리로 자동 분류.
3. **SYNC & VERIFY**: 변환 규칙에 따른 데이터 동기화 및 건수/해시 무결성 검증.
4. **CUTOVER GATE**: 관리자가 마이그레이션 검증 보고서를 확인 후 [Cutover 승인]을 클릭하면 Kanvas 위키 서비스가 정식 활성화됩니다.

---

## 3. Keycloak OIDC SSO 및 계정 관리

- **OIDC Discovery**: Keycloak Discovery 엔드포인트를 등록하고 Authorization Code + PKCE (S256) 인증을 켭니다.
- **Valid Redirect URI**: `https://kanvas.internal/api/v1/auth/oidc/callback`
- **Group mapping**: Keycloak 그룹을 Kanvas 계층 그룹으로 맵핑하여 자동 ACL 권한 할당.

---

## 4. API / MCP 키 관리 & 감사 로그 (Audit Log)

- **원자적 키 회전**: 보안 사고 발생 시 관리자 화면에서 [전체 개인 키 즉시 폐기] 기능을 실행하여 발급된 모든 API/MCP 키를 일괄 상실 처리할 수 있습니다.
- **감사 로그 (Audit Trail)**: 문서 삭제, ACL 변경, Migration Cutover 승인 등 모든 관리자 및 사용자 활동이 DB 감사 테이블에 무결성 보장 상태로 영구 보관됩니다.
