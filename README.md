<p align="center">
  <img src="docs/favicon.svg" alt="Kanvas Logo" width="90"><br><br>
  <h1 align="center">Kanvas</h1>
</p>

<p align="center">
  <strong>Confluence 호환 계층 및 온프레미스 에어갭 사내 지식 위키 플랫폼</strong><br>
  안전한 Confluence MySQL 마이그레이션과 원천 데이터 소유권을 단일 Docker 패키지로 제공합니다.
</p>

<p align="center">
  <a href="https://hkjang.github.io/kanvas/">🇰🇷 홍보 페이지</a> · <a href="https://hkjang.github.io/kanvas/index_en.html">🇺🇸 English Page</a> · <a href="https://hkjang.github.io/">🌐 전체 서비스</a> · <a href="https://github.com/sponsors/hkjang">💖 Sponsor</a>
</p>

---

## 현재 제공 기능

- 로컬 비상 관리자와 Keycloak/OIDC SSO, 그룹 기반 관리자 매핑
- Wiki Space, Page, 구조화 Editor JSON, 불변 Page Version, Comment, 검색
- User/Group 기반 Space·Page ACL을 REST API와 MCP에 동일 적용
- 개인화 영역과 개인 API/MCP 키 발급·폐기·원자적 회전
- 운영 현황 대시보드와 사용자 역할·상태, 그룹 멤버십, Space 보관을 관리하는 서비스 관리자 영역
- 사이트 이름·기준 URL·세션 만료 등 검증된 운영 설정, 감사 로그 검색·CSV 내보내기, 런타임 상태 화면
- Read-only Confluence MySQL Schema Discovery, Core/AO/Unknown Table 분류
- 재시작 가능한 Initial Snapshot: 사용자·그룹·멤버십·Space·페이지 전체 버전·댓글·첨부 메타데이터·ACL
- Confluence Storage XHTML → Canonical AST/Editor JSON 변환과 Macro 호환성·미지원 콘텐츠 리포트
- Snapshot과 분리해 반복 실행하는 정합성 재검증 Job, 예외 승인·해결·재오픈 및 감사 근거 관리
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
docker build -t kanvas-v0.4.0:latest .
```

## 문서

- [오프라인 설치 및 업그레이드](docs/OFFLINE_INSTALL.md)
- [관리자 운영 가이드](docs/ADMIN_GUIDE.md)
- [Confluence 마이그레이션 설계](docs/MIGRATION.md)
- [REST API와 MCP](docs/API_MCP.md)
- [보안 모델](docs/SECURITY.md)
- [릴리스 절차](docs/RELEASE.md)

API 원문은 실행 중인 서비스의 `/api/openapi.yaml`, MCP 엔드포인트는 `/mcp`에서 제공합니다.
