# Kanvas

Kanvas는 Confluence 호환 계층, 안전한 온라인 마이그레이션 제어면, PostgreSQL 기반 신규 Wiki를 하나의 제품으로 구성한 사내 지식 플랫폼입니다. Go 모듈러 모놀리스와 React SPA를 단일 Docker 이미지로 묶어 외부 CDN이나 인터넷 연결 없이 운영할 수 있습니다.

## 현재 제공 기능

- 로컬 비상 관리자와 Keycloak/OIDC SSO, 그룹 기반 관리자 매핑
- Wiki Space, Page, 구조화 Editor JSON, 불변 Page Version, Comment, 검색
- User/Group 기반 Space·Page ACL을 REST API와 MCP에 동일 적용
- 개인화 영역과 개인 API/MCP 키 발급·폐기·원자적 회전
- 서비스 관리자 영역, 암호화 설정, 감사 로그, 내장 상태 화면
- Read-only Confluence MySQL Schema Discovery, Core/AO/Unknown Table 분류
- 증거 기반 Migration 상태 머신과 Cutover Gate
- OpenAPI 문서와 8개 ACL-aware MCP Tool
- 로그인 화면과 프로필 컨텍스트 메뉴의 서비스 버전 표시
- 단일 비-root Docker 이미지와 오프라인 릴리스 자동화

상세한 구현 경계와 후속 단계는 [아키텍처 문서](docs/ARCHITECTURE.md)와 [마이그레이션 설계](docs/MIGRATION.md)를 참고하세요.

## 서비스 환경변수

Kanvas 컨테이너가 읽는 환경변수는 아래 네 개뿐입니다.

| 변수 | 용도 |
|---|---|
| `KANVAS_POSTGRES_DSN` | Kanvas PostgreSQL 연결 문자열 |
| `KANVAS_CONFLUENCE_DSN` | Read-only Confluence MySQL DSN. Discovery 전까지 빈 문자열 허용 |
| `KANVAS_BOOTSTRAP_ADMIN` | 최초 비상 관리자 사용자 이름 |
| `KANVAS_BOOTSTRAP_ADMIN_PASSWORD` | 최초 비상 관리자 암호. 최소 12자 |

Bootstrap 관리자는 관리자 계정이 하나도 없을 때만 생성됩니다. 이후 환경변수 변경으로 기존 암호가 덮어써지지 않습니다. OIDC, Attachment, Migration, Search 등 나머지 운영 설정은 Kanvas의 **서비스 관리** 화면에서 관리합니다.

## 개발 실행

```bash
cp .env.example .env
docker compose up --build
```

브라우저에서 `http://localhost:8080`에 접속합니다. 프로덕션에서는 TLS 종료 Reverse Proxy를 사용하고 `X-Forwarded-Proto`, `X-Forwarded-Host`를 전달해야 합니다.

## 빌드와 테스트

```bash
go test ./...
npm ci --prefix web
npm run build --prefix web
docker build -t kanvas-v0.1.0:latest .
```

## 문서

- [오프라인 설치 및 업그레이드](docs/OFFLINE_INSTALL.md)
- [관리자 운영 가이드](docs/ADMIN_GUIDE.md)
- [Confluence 마이그레이션 설계](docs/MIGRATION.md)
- [REST API와 MCP](docs/API_MCP.md)
- [보안 모델](docs/SECURITY.md)
- [릴리스 절차](docs/RELEASE.md)

API 원문은 실행 중인 서비스의 `/api/openapi.yaml`, MCP 엔드포인트는 `/mcp`에서 제공합니다.
