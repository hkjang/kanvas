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

## 단계별 구현 경계

Kanvas v0.1의 Migration Center는 Discovery와 안전한 evidence gate를 제공합니다. Snapshot converter, binlog CDC reader, attachment binary copier, rendering comparator, legacy replay worker는 schema와 job/check boundary만 정의되어 있으며 production migration 전에 별도 구현과 실제 Confluence 6.9.1 golden dataset 검증이 필요합니다. 미완료 Job evidence가 없으면 상태 머신이 다음 단계로 진행하지 않습니다.

이 제약은 UI가 허위 Readiness나 Cutover 성공을 표시하지 않도록 하기 위한 fail-closed 동작입니다.
