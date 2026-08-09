# 보안 모델

## Secret

- OIDC Client Secret과 secret setting은 AES-256-GCM으로 암호화합니다.
- Master key는 PostgreSQL과 분리된 `/var/lib/kanvas/master.key`에 mode `0600`으로 생성합니다.
- PostgreSQL/Confluence DSN 전체 문자열을 로그나 관리자 응답에 출력하지 않습니다. 화면에는 설정 여부와 짧은 fingerprint만 표시합니다.
- 개인 API key는 SHA-256 hash와 prefix만 저장합니다.

## Web

- HttpOnly, SameSite=Lax session cookie
- state-changing browser request의 CSRF token 검증
- OIDC state와 nonce, ID token issuer/audience/signature 검증
- CSP, frame deny, MIME sniffing 방지, same-origin referrer policy
- 요청 body 상한과 unknown JSON field 거부
- local login IP별 in-memory rate limit
- non-root container runtime

## Authorization

권한은 server에서 최종 판정합니다. Page 제한과 Space 제한을 모두 만족해야 하며 user/group membership을 지원합니다. Admin은 Wiki ACL을 우회할 수 있지만 관리자 API는 administrator browser session만 허용합니다.

## 운영 권고

- Bootstrap password와 DB password는 별도로 관리하고 12자보다 긴 무작위 값을 사용합니다.
- Confluence DB account에는 SELECT만 부여합니다.
- TLS 없이 사내망에 직접 노출하지 않습니다.
- PostgreSQL backup과 Kanvas data volume을 한 복구 단위로 관리합니다.
- OIDC administrator group 변경, key 회전, cutover 작업은 감사 로그에서 재확인합니다.
