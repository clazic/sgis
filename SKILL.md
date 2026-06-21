---
name: sgis
description: SGIS(통계지리정보서비스) Open API로 한국 지오공간·행정통계 데이터를 조회·시각화하는 CLI 도구. "인구통계 조회", "행정구역 경계 GeoJSON", "주소 지오코딩", "시도 코드 목록", "사업체 통계", "가구통계", "주택통계", "격자경계", "좌표변환", "행정구역경계 다운로드", "통계지리 조회" 등 지오공간·통계 요청 시 사용.
user-invocable: true
argument-hint: "<지역코드|행정구역명|통계유형|주소|명령>"
---

# SGIS CLI

## 학습 루프 (AI 필독 — 스킬이 진화하는 방식)

**SGIS 작업을 시작하면 가장 먼저 [LEARNINGS.md](LEARNINGS.md)를 읽는다.** 과거의 실패·함정·성공 쿼리가 누적된 "오답노트"다.

- **조회 전 READ**: `LEARNINGS.md`의 `엔드포인트별 함정`·`오류코드 대응`을 확인해 같은 실수를 반복하지 않는다.
- **작업 후 APPEND**: 예상 못한 실패나 처음 통한 쿼리·해결한 함정을 겪으면 `LEARNINGS.md`의 해당 섹션에 **한 줄 이상** 추가한다("왜"를 반드시 포함). 이렇게 쓸수록 스킬이 똑똑해진다.

---

## First-Run Onboarding (AI 필독)

SGIS 명령 실행 전 반드시 다음 순서를 따를 것:

0. **바이너리 확인**: PATH에 `sgis`가 없으면 (`command -v sgis` 또는 `where sgis` 실패 시):
   - macOS/Linux: `curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.sh | sh`
   - Windows: `irm https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.ps1 | iex`

1. `sgis config get` 실행 → 자격증명 존재 여부 확인

2. `consumer_key` / `consumer_secret`이 없고 환경변수 `SGIS_CONSUMER_KEY` / `SGIS_CONSUMER_SECRET`도 없으면 → **실행 중단 후 안내:**
   - "SGIS API 자격증명이 필요합니다."
   - "https://sgis.mods.go.kr/developer/html/newOpenApi/guide/guide/getApiKey.html 에서 발급 후 알려주시면 등록합니다."

3. 사용자가 consumerKey + consumerSecret 제공 → 다음 명령 자동 실행:
   ```bash
   sgis config set-credential <KEY> <SECRET>
   ```

4. 검증 — 테스트 호출:
   ```bash
   sgis code stage
   ```
   시도 목록(17개)이 나오면 정상. 오류 시 키/시크릿 재확인 요청.

5. 검증 실패 시 → 오류 메시지 확인 후 발급 URL 재안내.

> **accessToken은 자동 관리됨** — consumerKey/consumerSecret을 설정하면 CLI가 4시간짜리 토큰을 자동 발급·캐싱·갱신한다. 사용자가 토큰을 직접 다룰 필요 없음.

---

## 설치

| 방법 | 명령 |
|------|------|
| macOS / Linux | `curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.sh \| sh` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.ps1 \| iex` |

## 업데이트

```bash
sgis update          # 최신 버전으로 업데이트 (바이너리 + 스킬 파일 함께)
sgis update --check  # 업데이트 확인만 (설치 안 함)
sgis update --force  # 같은 버전이어도 강제 재설치
```

GitHub 최신 릴리스에서 OS별 바이너리와 스킬 파일(SKILL.md, references/, LEARNINGS.md, templates 등)을 함께 내려받아 `~/.claude/skills/sgis`(및 `~/.codex/skills/sgis`)에 반영합니다. macOS/Linux/Windows 모두 지원합니다.

---

## 빠른 시작

```bash
sgis config set-credential <CONSUMER_KEY> <CONSUMER_SECRET>  # 자격증명 등록

sgis code stage                          # 시도 코드 목록
sgis data population --adm-cd 11 --year 2020    # 서울 인구통계
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson   # 서울 행정구역 경계
sgis geocode address "서울특별시 종로구 청와대로 1"   # 주소 → 좌표
```

> API 자격증명 발급: https://sgis.mods.go.kr/developer/html/newOpenApi/guide/guide/getApiKey.html

---

## 환경

**배포 바이너리 경로:**

| OS / ARCH | 경로 |
|-----------|------|
| macOS / Linux | `~/.local/bin/sgis` |
| Windows amd64 | `%LOCALAPPDATA%\Programs\sgis\sgis.exe` |

> `apps/` 디렉토리는 개발/CI 빌드 전용입니다. 배포 바이너리는 위 경로에 설치됩니다.

**빌드 (개발 시 — 항상 멀티플랫폼):**

```bash
cd src
make build      # 5개 플랫폼 빌드 후 src/bin/ 과 apps/ 두 곳에 자동 배치
```

> 모든 타깃은 `CGO_ENABLED=0` — SQLite/Parquet 미사용으로 순수 Go 크로스컴파일.

**바이너리 탐색 순서 (실행 시):**

| 순서 | 경로 |
|------|------|
| 1. PATH | `command -v sgis` / `where sgis` (설치 후 기본 경로) |
| 2. 개발 빌드 | `src/bin/sgis-<os>-<arch>` (로컬 빌드 시) |
| 3. 안내 | PATH에 없으면 `install.sh` / `install.ps1`로 설치 안내 |

**설정/캐시 경로:**

| 항목 | 경로 | 비고 |
|------|------|------|
| 설정 | `~/.sgis/config.yaml` | consumerKey, consumerSecret, 기본 포맷 |
| 토큰 캐시 | `~/.sgis/token.json` | accessToken 자동 관리 (0o600) |
| 응답 캐시 | `~/.sgis/cache/` | 메타/코드 응답만, TTL 설정 가능 |

> Windows: `~` → `%USERPROFILE%`, 한글 깨짐 시 `chcp 65001`  
> 환경변수 `SGIS_CONSUMER_KEY` / `SGIS_CONSUMER_SECRET` 설정 시 config.yaml보다 우선 적용

---

## 명령어

### data — 통계 조회

| 서브커맨드 | 설명 | 주요 플래그 |
|-----------|------|------------|
| `sgis data population` | 인구통계 | `--adm-cd`, `--year`, `--low-search` |
| `sgis data household` | 가구통계 | `--adm-cd`, `--year`, `--low-search` |
| `sgis data house` | 주택통계 | `--adm-cd`, `--year`, `--house-type` |
| `sgis data company` | 사업체통계 | `--adm-cd`, `--year`, `--class-code` |
| `sgis data searchpopulation` | 인구검색(조건별) | `--adm-cd`, `--year`, `--gender` |
| `sgis data farmhousehold` | 농가통계 | `--adm-cd`, `--year` |
| `sgis data forestryhousehold` | 임가통계 | `--adm-cd`, `--year` |
| `sgis data fisheryhousehold` | 어가통계 | `--adm-cd`, `--year`, `--oga-div` |
| `sgis data householdmember` | 가구원통계 | `--adm-cd`, `--year`, `--data-type` |

```bash
# 서울 2020년 인구 (시도 단위)
sgis data population --adm-cd 11 --year 2020

# 경기도 시군구별 사업체 통계
sgis data company --adm-cd 31 --year 2023 --low-search 1

# 전국 주택통계 JSON 출력
sgis data house --year 2023 --format json
```

**공통 파라미터:**
- `--adm-cd`: 미지정=전국, 2자리=시도, 5자리=시군구, 8자리=읍면동
- `--low-search`: 0=해당코드만, 1=1단계 하위, 2=2단계 하위
- `--year`: 통계 기준연도 (엔드포인트별 지원 연도 다름; `sgis code year-data` 확인)

### boundary — 경계 GeoJSON 조회

| 서브커맨드 | 설명 | 주요 플래그 |
|-----------|------|------------|
| `sgis boundary hadmarea` | 행정구역 경계 | `--adm-cd`, `--year`, `--low-search` |
| `sgis boundary statsarea` | 집계구 경계 | `--adm-cd` (8자리) |
| `sgis boundary userarea` | 영역내 경계 | `--minx`, `--miny`, `--maxx`, `--maxy`, `--cd` |
| `sgis boundary urbanboundary` | 도시/준도시 경계 | (도시권 파라미터) |
| `sgis boundary grid` | 행정구역 격자경계 | (격자 파라미터) |
| `sgis boundary buildingarea` | 건물경계 | `accessToken` |
| `sgis boundary floorboundary` | 층별 최외각 공간속성 | `accessToken` |
| `sgis boundary floorcompany` | 층별 사업체 공간속성 | `accessToken` |

```bash
# 서울 2025년 행정구역 경계 GeoJSON
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson

# GeoJSON 파일로 저장
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson -o seoul.geojson
```

> **좌표계 주의**: 경계 좌표는 **UTM-K (EPSG:5179)** 기본. Leaflet 지도(WGS84 기대) 사용 시 좌표 변환 필요.

### geocode — 지오코딩 / 좌표변환

| 서브커맨드 | 설명 | 좌표계 |
|-----------|------|--------|
| `sgis geocode address` | 주소 → 좌표 (UTM-K) | 출력 UTM-K |
| `sgis geocode address-wgs84` | 주소 → 좌표 (WGS84) | 출력 WGS84 |
| `sgis geocode reverse` | 좌표 → 주소 (UTM-K 입력) | 입력 UTM-K |
| `sgis geocode reverse-wgs84` | 좌표 → 주소 (WGS84 입력) | 입력 WGS84 |
| `sgis geocode transform` | 좌표계 변환 | `--src`, `--dst` 지정 |

```bash
# 주소 지오코딩 (UTM-K 좌표 반환)
sgis geocode address "서울특별시 종로구 청와대로 1"

# WGS84 좌표 반환
sgis geocode address-wgs84 "부산광역시 해운대구 우동 1413"

# UTM-K → WGS84 좌표 변환
sgis geocode transform --src 5179 --dst 4326 --pos-x 953932 --pos-y 1952053
```

### code — 행정구역 코드 조회

| 서브커맨드 | 설명 |
|-----------|------|
| `sgis code stage` | 시도/시군구/읍면동 코드 조회 |
| `sgis code findcodeinsmallarea` | 소지역(집계구) 코드 찾기 |
| `sgis code year-data` | 가용 기준연도 목록 |
| `sgis code industrycode` | 산업분류 코드 |

```bash
sgis code stage                  # 시도 17개 목록
sgis code stage --cd 11          # 서울 시군구 목록
sgis code stage --cd 11010       # 종로구 읍면동 목록
sgis code year-data              # 통계 가용 연도 목록
sgis code industrycode --class-deg 10   # 산업분류 코드
```

### config — 설정 관리

| 명령 | 설명 |
|------|------|
| `sgis config set-credential <KEY> <SECRET>` | consumerKey/consumerSecret 등록 |
| `sgis config get` | 현재 설정 표시 (시크릿 마스킹) |

### update — 자동 업데이트

```bash
sgis update           # 최신 버전 업데이트
sgis update --check   # 버전 확인만
sgis update --force   # 강제 재설치
```

---

## 출력 포맷

| 포맷 | 플래그 | 용도 |
|------|--------|------|
| `table` | `--format table` | 터미널 테이블 (기본) |
| `json` | `--format json` | JSON — AI 파이프라인, jq 처리 |
| `csv` | `--format csv` | CSV — 스프레드시트 |
| `geojson` | `--format geojson` | GeoJSON — 지도/공간분석 (boundary 전용) |
| `xlsx` | `--format xlsx` | Excel 파일 (`-o` 필수) |

```bash
sgis data population --adm-cd 11 --year 2020 --format json
sgis data population --adm-cd 11 --year 2020 --format xlsx -o 서울인구.xlsx
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson -o seoul.geojson
```

---

## 인증 흐름

```
consumerKey + consumerSecret
        │
        ▼  (자동, CLI 내부)
  SGIS 인증 엔드포인트
  /OpenAPI3/auth/authentication.json
        │
        ▼
  accessToken (유효기간 4시간)
  → ~/.sgis/token.json 캐시 (0o600)
        │
        ▼  (모든 API 호출 시 자동 첨부)
  ?accessToken=<TOKEN>
```

- 토큰 만료 전 자동 갱신 (만료 ~24분 전)
- 병렬 실행 시 파일 락으로 중복 발급 방지
- `SGIS_CONSUMER_KEY` / `SGIS_CONSUMER_SECRET` 환경변수로 CI 지원

---

## 핵심 규칙

- **adm_cd 자릿수 확인** — 2자리=시도, 5자리=시군구, 8자리=읍면동. 자릿수가 틀리면 빈 결과 또는 전국 반환
- **year 범위 확인** — 엔드포인트마다 지원 연도가 다름. `sgis code year-data` 또는 `sgis data --help`로 먼저 확인
- **좌표계 주의** — 기본 좌표는 **UTM-K(EPSG:5179)**. WGS84(GPS/지도 앱)가 필요하면 `*wgs84` 엔드포인트 사용
- **low_search 기본값** — 미지정 시 엔드포인트별 기본값 다름. 하위 단위 조회 시 `--low-search 1` 명시
- **boundary는 geojson 포맷** — `--format geojson` 없이도 동작하지만 GeoJSON이 원본 형식
- **에러 즉시 보고** — 오류 발생 시 숨기지 말고 errCd + errMsg 사용자에게 안내

---

## 상세 문서

| 문서 | 내용 |
|------|------|
| [01-installation.md](references/01-installation.md) | 설치 및 초기 자격증명 설정 |
| [02-auth.md](references/02-auth.md) | 인증 흐름 (consumerKey→accessToken 자동 관리) |
| [03-commands.md](references/03-commands.md) | 명령어 레퍼런스 (4그룹 + config + update) |
| [sgis-endpoints.md](references/sgis-endpoints.md) | 엔드포인트 전체 카탈로그 (90개, canonical 기록) |
| [region-codes.md](references/region-codes.md) | 행정구역 코드표 (시도 17개 + 시군구 전체, 라이브 실측) |
| [LEARNINGS.md](LEARNINGS.md) | **학습 로그(오답노트)** — 실패·함정·성공 쿼리 누적 |
