# REST API와 MCP

## REST

OpenAPI 3.1 문서는 `/api/openapi.yaml`에서 제공합니다. Browser는 session cookie와 `X-CSRF-Token`, 자동화 client는 개인 Bearer key를 사용합니다.

```bash
curl -H 'Authorization: Bearer knv_REDACTED' \
  'https://kanvas.example/api/v1/search?q=runbook'
```

개인 키는 프로필 → 개인화 및 키 관리 → API 및 MCP 키에서 발급합니다. 평문 token은 발급 또는 회전 직후 한 번만 표시됩니다.

관리자 Browser session에서는 다음 Migration 운영 API도 제공합니다. 상태 변경 요청은 `X-CSRF-Token`이 필요하며 개인 API key로는 호출할 수 없습니다.

| Endpoint | 기능 |
|---|---|
| `POST /api/v1/admin/migration/reconciliation` | 최신 완료 Snapshot 기반 독립 정합성 Job 시작 |
| `GET /api/v1/admin/migration/unsupported` | 최신 Snapshot 예외 검색·필터·페이지 조회 |
| `PATCH /api/v1/admin/migration/unsupported/{itemId}` | 단일 예외 승인·해결·재오픈 |
| `POST /api/v1/admin/migration/unsupported/bulk` | 최대 500개 예외의 원자적 일괄 결정 |

`APPROVED`와 `RESOLVED`에는 최대 2,000자의 조치 근거가 필수이며 관리자, 처리 시각, 상태 변경이 감사 로그에 기록됩니다.

## MCP

Endpoint는 `POST /mcp`이며 Streamable HTTP JSON-RPC 요청을 받습니다.

```bash
curl -X POST https://kanvas.example/mcp \
  -H 'Authorization: Bearer knv_REDACTED' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}'
```

제공 tool:

| Tool | Scope |
|---|---|
| `search_pages` | `wiki:read` |
| `get_page` | `wiki:read` |
| `list_spaces` | `wiki:read` |
| `get_page_history` | `wiki:read` |
| `get_comments` | `wiki:read` |
| `create_page` | `wiki:write` |
| `update_page` | `wiki:write` |
| `add_comment` | `wiki:write` |

Tool 결과는 API key owner의 Space/Page ACL로 필터링됩니다. Update는 current version을 요구하고 충돌 시 실패합니다.
