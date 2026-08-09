# 관리자 운영 가이드

일반 사용자의 **개인화 및 키 관리**와 관리자의 **Kanvas 서비스 관리**는 프로필 메뉴에서 명확히 분리됩니다.

## Keycloak OIDC

서비스 관리 → 인증 및 SSO에서 다음을 입력합니다.

- Issuer: `https://keycloak.example/realms/company`
- Client ID: `kanvas`
- Client Secret
- Groups Claim: 기본 `groups`
- 관리자 그룹: 기본 `kanvas-admins`
- 최초 로그인 자동 프로비저닝 여부

Keycloak Client의 redirect URI는 아래와 같습니다.

```text
https://<kanvas-host>/api/v1/auth/oidc/callback
```

Issuer의 `.well-known/openid-configuration`을 이용해 authorization/token/JWKS endpoint를 자동으로 찾습니다. ID token의 issuer, audience, signature, nonce, state를 검증합니다. 관리자 그룹 claim이 일치하는 사용자는 `ADMIN`으로 동기화됩니다.

로컬 Bootstrap 관리자는 Keycloak 장애 시 사용하는 Emergency account입니다. 환경변수 암호는 최초 생성에만 사용되며 기존 관리자를 덮어쓰지 않습니다.

## 운영 설정

서비스 관리 → 데이터 원본에서 Attachment root, migration batch size, parallelism을 설정할 수 있습니다. PostgreSQL test DSN은 연결 테스트에만 사용하고 저장하거나 로그에 남기지 않습니다.

## 감사

다음 항목은 `audit_events`에 별도 기록합니다.

- Local/OIDC login과 실패, logout
- Space/Page/Comment 생성과 Page 편집
- 개인 API key 생성, 회전, 폐기
- OIDC와 서비스 설정 변경
- Schema Discovery와 migration phase 변경

관리자 REST API는 Browser administrator session과 CSRF token을 요구합니다. 개인 API key로는 접근할 수 없습니다.
