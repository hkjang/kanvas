# 오프라인 설치

GitHub Release에서 `kanvas-v<버전>.tar.gz` 하나를 내려받아 망 반입 절차에 따라 격리망으로 이동합니다. 이 파일은 `docker save` 결과를 gzip으로 압축한 것이며 애플리케이션, UI, CA 인증서, timezone 데이터가 포함됩니다.

## 1. 이미지 적재

```bash
docker load < kanvas-v0.4.0.tar.gz
docker image inspect kanvas-v0.4.0:latest
```

## 2. 영구 볼륨

```bash
docker volume create kanvas-data
```

이 볼륨에는 설정 비밀값을 암호화하는 master key가 들어갑니다. 볼륨을 잃으면 암호화된 OIDC Client Secret을 복구할 수 없으므로 PostgreSQL backup과 함께 보관하십시오.

## 3. 실행

PostgreSQL은 사내 표준 인스턴스를 준비합니다. Kanvas DB user에는 대상 schema의 CREATE, SELECT, INSERT, UPDATE, DELETE 권한이 필요합니다.

```bash
docker run -d \
  --name kanvas \
  --restart unless-stopped \
  -p 8080:8080 \
  -v kanvas-data:/var/lib/kanvas \
  -e KANVAS_POSTGRES_DSN='postgres://kanvas:REDACTED@postgres.internal:5432/kanvas?sslmode=require' \
  -e KANVAS_CONFLUENCE_DSN='readonly:REDACTED@tcp(mysql.internal:3306)/confluence?charset=utf8mb4&parseTime=true' \
  -e KANVAS_BOOTSTRAP_ADMIN='admin' \
  -e KANVAS_BOOTSTRAP_ADMIN_PASSWORD='REPLACE-WITH-LONG-EMERGENCY-PASSWORD' \
  kanvas-v0.4.0:latest
```

Confluence를 아직 연결하지 않는 경우 `KANVAS_CONFLUENCE_DSN=''`를 전달합니다. DSN은 Go MySQL driver 형식이며 source account에는 SELECT 권한만 부여합니다.

## 4. 상태 확인

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
```

`healthz`는 프로세스 상태, `readyz`는 PostgreSQL 연결까지 확인합니다.

## 5. TLS Proxy

Kanvas 자체 listener는 고정 포트 8080입니다. 사내 Reverse Proxy에서 TLS를 종료하고 다음 header를 전달합니다.

```text
Host
X-Forwarded-Proto: https
X-Forwarded-Host: wiki.company.internal
X-Forwarded-For
```

## 업그레이드

1. PostgreSQL과 `kanvas-data` volume을 backup합니다.
2. 새 `kanvas-v<버전>.tar.gz`를 `docker load` 합니다.
3. 기존 컨테이너를 중지하되 volume은 삭제하지 않습니다.
4. 같은 네 환경변수와 volume으로 새 이미지 컨테이너를 시작합니다.
5. `/readyz`, 로그인 화면 버전, 관리자 서비스 상태를 확인합니다.

이미지는 non-root `kanvas` 사용자로 실행됩니다. `/var/lib/kanvas` 외의 쓰기 권한은 필요하지 않습니다.
