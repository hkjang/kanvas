# REST API와 MCP

## REST

OpenAPI 3.1 문서는 `/api/openapi.yaml`에서 제공합니다. Browser는 session cookie와 `X-CSRF-Token`, 자동화 client는 개인 Bearer key를 사용합니다.

```bash
curl -H 'Authorization: Bearer knv_REDACTED' \
  'https://kanvas.example/api/v1/search?q=runbook'
```

개인 키는 프로필 → 개인화 및 키 관리 → API 및 MCP 키에서 발급합니다. 평문 token은 발급 또는 회전 직후 한 번만 표시됩니다.

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
