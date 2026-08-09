# Kanvas 아키텍처

## 제품 경계

Kanvas는 런타임 배포 단위를 하나로 유지하는 Modular Monolith입니다. UI와 API는 논리적으로 분리하지만 React 정적 자산을 Go 바이너리에 포함합니다. 외부 필수 구성요소는 PostgreSQL 하나이며 Redis, 외부 Search, CDN은 요구하지 않습니다.

```text
Browser ── REST ─┐
MCP Client ──────┼─> Auth + ACL ─> Wiki Domain ─> PostgreSQL
API Client ──────┘          │
                            ├─> Audit
Admin ─> Migration Center ──┼─> Read-only MySQL Inspector
                            └─> Cutover Gate
```

## 모듈

| 패키지 | 책임 |
|---|---|
| `internal/auth` | Local session, CSRF, OIDC Discovery/Callback, API key identity |
| `internal/store` | Canonical PostgreSQL schema와 Repository |
| `internal/migration` | Schema Discovery, Snapshot, 독립 Reconciliation, 예외 정책, state machine과 evidence gate |
| `internal/mcp` | MCP JSON-RPC와 ACL-aware Wiki tools |
| `internal/api` | REST, admin/personal boundary, security headers |
| `web` | React Wiki, editor, personal, service admin UI |
| `webembed` | 빌드된 SPA를 Go binary에 embed |

## 데이터 원본 불변성

Confluence DSN은 `database/sql`의 MySQL read path에서만 사용합니다. Legacy 연결로 실행하는 SQL은 `SELECT`, `SHOW`, `information_schema` 조회뿐입니다. 신규 페이지 작성과 설정 변경은 PostgreSQL에만 수행합니다.

## Canonical page

`pages`는 현재 메타데이터와 current version 포인터를 보관합니다. 매번 게시할 때 `page_versions`에 새 immutable row를 만들고, `legacy_storage`, `editor_document`, `rendered_text`, `content_hash`를 분리합니다. 모든 update는 기대 버전을 요구하므로 동시 수정 손실을 막습니다.

## 인증과 권한

Browser는 HttpOnly session cookie와 CSRF token을 사용합니다. API/MCP는 개인 Bearer key를 사용합니다. 두 경로 모두 같은 `User` identity로 수렴한 다음 Store의 Space/Page ACL 판정을 거칩니다. 관리자 API는 개인 키로 호출할 수 없고 browser 관리자 session만 허용합니다.

## 설정

부팅에 필요한 PostgreSQL과 Bootstrap 정보만 환경변수입니다. 운영 설정은 `system_settings`에 저장하며 secret 값은 `/var/lib/kanvas/master.key`의 AES-256-GCM key로 암호화합니다. 데이터 볼륨과 PostgreSQL backup은 함께 보존해야 합니다.
