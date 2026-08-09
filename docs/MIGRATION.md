# Confluence 마이그레이션

## 원칙

1. Legacy MySQL에는 DML을 실행하지 않습니다.
2. 실제 설치 DB를 Discovery한 결과를 Migration Profile의 기준으로 사용합니다.
3. 원본 XHTML, Legacy ID, Attachment hash, Permission equivalence를 보존·검증합니다.
4. 상태 문자열을 바꾸는 것만으로 다음 단계에 진입할 수 없습니다. 완료된 Job과 정합성 Check가 증거로 존재해야 합니다.
5. Cutover는 최소 13개 필수 check가 모두 `PASS` 또는 `APPROVED`일 때만 허용합니다.

## 상태

```text
LEGACY → DISCOVERY → SNAPSHOT → CDC_SYNC → VERIFY → SHADOW
       → CUTOVER_READY → FREEZE → FINAL_SYNC → CUTOVER
       → STABILIZING → COMPLETE
```

실패는 `ERROR`, 전환 후 복귀는 `WINBACK`으로 기록합니다. `source_mode`는 `LEGACY`, `SHADOW`, `POSTGRES` 중 하나입니다.

## Discovery

현재 구현된 Inspector는 아래 정보를 source MySQL에서 read-only로 수집합니다.

- MySQL version, database charset, collation
- 전체 table, approximate row count, data+index size, engine
- Core, `AO_*` plugin, Unknown table 분류
- `CONTENT`, `BODYCONTENT`, `SPACES`, Permission, CWD, Label 등 core count
- `CONFVERSION` 기반 Confluence build 식별
- 결과와 실행 감사 기록

## Initial Snapshot

Kanvas는 Discovery 완료 후 아래 데이터를 실제 Confluence MySQL에서 읽어 PostgreSQL canonical schema로 이관합니다.

- `CWD_USER`, `CWD_GROUP`, `CWD_MEMBERSHIP`
- `SPACES`, `CONTENT`, `BODYCONTENT` 및 전체 페이지 버전 체인
- 댓글과 첨부파일 메타데이터
- `SPACEPERMISSIONS`, `CONTENT_PERM_SET`, `CONTENT_PERM`
- 원본 Storage XHTML, Canonical AST, Editor JSON, 검색용 텍스트, SHA-256 content hash
- 페이지 트리, 내부 링크, Macro 사용 현황과 Unsupported Content

Job은 레코드별 상태·체크포인트·오류를 저장하며 취소 또는 프로세스 재시작 후 재개할 수 있습니다. 완료된 레코드는 건너뛰고 실패 레코드만 다시 처리합니다. 같은 Snapshot을 반복 실행해도 Legacy ID 기반 UPSERT로 중복 객체를 만들지 않습니다. 주체를 확인할 수 없는 권한은 공개로 추정하지 않고 실패시켜 정보 노출을 방지합니다.

## 독립 정합성 재검증

완료된 최신 Snapshot을 근거로 별도 `RECONCILIATION` Job을 실행할 수 있습니다. 원본을 다시 복사하지 않고 Snapshot 레코드와 PostgreSQL canonical 데이터를 비교해 사용자, 그룹, Space, 페이지, 버전, 댓글, 첨부 메타데이터, 권한, 콘텐츠 hash, 링크와 Macro check를 다시 계산합니다.

- 동시 재검증 Job은 데이터베이스의 부분 Unique Index로 하나만 허용합니다.
- 실행·완료·실패·취소 상태와 근거 Snapshot ID, check 수, readiness를 Job과 감사 로그에 남깁니다.
- 상태 전환은 advisory transaction lock 안에서 현재 단계와 근거를 함께 검증해 경쟁 요청을 직렬화합니다.

## Unsupported Content Center

최신 Snapshot에서 발견한 `UNKNOWN_MACRO`, `INVALID_XHTML`, `UNKNOWN_PLUGIN_DATA`, `ORPHAN_PAGE`를 검색·상태·유형별로 검토합니다. 관리자는 최대 500건을 일괄 처리할 수 있습니다.

- `OPEN`: 미해결 상태. Macro는 `WARNING`, 고아 페이지는 `FAIL`이며 Cutover 근거로 인정하지 않습니다.
- `APPROVED`: 변환되지 않은 잔여 위험을 명시적으로 수용합니다. 사유와 관리자, 시간이 필수로 기록됩니다.
- `RESOLVED`: 수동 수정 또는 변환 조치가 끝난 상태입니다. 조치 근거가 필수입니다.
- 승인·해결 항목은 언제든 `OPEN`으로 재개할 수 있으며 관련 check와 readiness가 즉시 재계산됩니다.

위험 승인은 실제 변환 성공을 의미하지 않습니다. 모든 결정은 감사 이벤트로 남고 Cutover Gate에서는 `PASS`와 `APPROVED`를 구분해 표시합니다.

## v0.4 구현 경계

Attachment binary copier, MySQL binlog CDC reader, rendering comparator, Shadow Read, Cutover/Winback replay worker는 아직 구현되지 않았습니다. 따라서 첨부 바이너리가 있으면 hash 검사가 `WARNING`이고, 미지원 Macro도 승인 전까지 `WARNING`입니다. `SNAPSHOT → CDC_SYNC` 전환은 CDC 엔진이 제공될 때까지 서버에서 명시적으로 거부합니다.

이 제약은 UI가 허위 Readiness나 Cutover 성공을 표시하지 않도록 하기 위한 fail-closed 동작입니다.
