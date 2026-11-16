# sgis

SGIS(통계지리정보서비스) Open API CLI 도구 — 행정구역 경계·인구·지오코딩 데이터를 터미널에서 조회·시각화합니다.

```bash
sgis code stage                                        # 시도 코드 목록
sgis data population --adm-cd 11 --year 2020           # 서울 인구통계
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson  # 서울 경계 GeoJSON
sgis geocode geocode --address "서울특별시 종로구 청와대로 1"  # 주소 → 좌표
```

---

## 설치

> 모든 방법은 **sudo/관리자 권한 없이** user scope에 설치됩니다.

### macOS / Linux (curl)

```bash
curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.sh | sh
```

설치 위치:
- 스킬 파일: `~/.claude/skills/sgis/`, `~/.codex/skills/sgis/`
- 바이너리: `~/.claude/skills/sgis/apps/sgis-<os>-<arch>`
- PATH 등록: `~/.local/bin/sgis` symlink

PATH가 등록되지 않은 경우 다음을 `~/.zshrc` 또는 `~/.bashrc`에 추가:
```bash
export PATH="$HOME/.local/bin:$PATH"
```

특정 버전 설치:
```bash
SGIS_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.ps1 | iex
```

설치 위치:
- 스킬 파일: `%USERPROFILE%\.claude\skills\sgis\`, `%USERPROFILE%\.codex\skills\sgis\`
- 바이너리: `%LOCALAPPDATA%\Programs\sgis\sgis.exe`
- PATH: User PATH에 자동 등록 (새 터미널에서 적용)

특정 버전 설치:
```powershell
$env:SGIS_VERSION="v0.1.0"
irm https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.ps1 | iex
```

**Windows 트러블슈팅:**

| 증상 | 해결 방법 |
|------|-----------|
| 실행 정책 오류 | `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser` |
| 한글 깨짐 | `chcp 65001` 실행 후 터미널 재시작 |
| `sgis` 명령 인식 안 됨 | 새 터미널 창 열기 (PATH 갱신 필요) |

---

## 자격증명 설정

SGIS Open API **서비스 ID**(consumerKey) / **보안 Key**(consumerSecret)가 필요합니다.  
[https://sgis.mods.go.kr/developer/html/newOpenApi/guide/guide/getApiKey.html](https://sgis.mods.go.kr/developer/html/newOpenApi/guide/guide/getApiKey.html) 에서 무료 발급.

```bash
# 직접 입력
sgis config set-credential <서비스 ID> <보안 Key>

# 환경변수 (CI/서버 환경)
export SGIS_CONSUMER_KEY="<서비스 ID>"         # macOS/Linux
export SGIS_CONSUMER_SECRET="<보안 Key>"
$env:SGIS_CONSUMER_KEY = "<서비스 ID>"         # Windows PowerShell
$env:SGIS_CONSUMER_SECRET = "<보안 Key>"

# 설정 확인
sgis config get
```

> accessToken은 자동 발급·캐싱·갱신됩니다. 토큰을 직접 관리할 필요 없습니다.

---

## 빠른 시작

```bash
# 행정구역 코드 조회
sgis code stage                          # 시도 17개 목록
sgis code stage --cd 11                  # 서울 시군구 목록
sgis code stage --cd 11010               # 종로구 읍면동 목록

# 인구통계
sgis data population --adm-cd 11 --year 2020
sgis data population --adm-cd 31 --year 2023 --low-search 1   # 경기도 시군구별

# 행정구역 경계 (GeoJSON)
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson -o seoul.geojson

# 지오코딩
sgis geocode geocode --address "부산광역시 해운대구 우동 1413"
sgis geocode geocodewgs84 --address "서울특별시 중구 태평로1가 31"   # WGS84 좌표

# 업데이트 확인
sgis update --check
```

---

## 주요 명령어

| 그룹 | 명령어 | 설명 |
|------|--------|------|
| **data** | `sgis data population` | 인구통계 |
| | `sgis data household` | 가구통계 |
| | `sgis data house` | 주택통계 |
| | `sgis data company` | 사업체통계 |
| | `sgis data farmhousehold` | 농가통계 |
| | `sgis data forestryhousehold` | 임가통계 |
| | `sgis data fisheryhousehold` | 어가통계 |
| | `sgis data householdmember` | 가구원통계 |
| | `sgis data searchpopulation` | 인구검색(조건별) |
| **boundary** | `sgis boundary hadmarea` | 행정구역 경계 GeoJSON |
| | `sgis boundary statsarea` | 집계구 경계 GeoJSON |
| | `sgis boundary userarea` | 영역내 경계 GeoJSON |
| | `sgis boundary urban-boundary` | 도시/준도시 경계 GeoJSON |
| | `sgis boundary grid-data` | 격자 경계 GeoJSON |
| | `sgis boundary figure-buildingarea` | 전개도 건물경계 GeoJSON |
| | `sgis boundary figure-floorboundary` | 층별 최외각 공간속성 |
| | `sgis boundary figure-floorcompany` | 층별 사업체 공간속성 |
| **geocode** | `sgis geocode geocode` | 주소 → 좌표 (UTM-K) |
| | `sgis geocode geocodewgs84` | 주소 → 좌표 (WGS84) |
| | `sgis geocode rgeocode` | 좌표 → 주소 (UTM-K 입력) |
| | `sgis geocode rgeocodewgs84` | 좌표 → 주소 (WGS84 입력) |
| | `sgis geocode transcoord` | 좌표계 변환 |
| **search** | `sgis search relword` | 연관어검색 (검색어→유의어) |
| | `sgis search sop` | SOP검색 (검색어→통계 SOP) |
| **code** | `sgis code stage` | 시도/시군구/읍면동 코드 |
| | `sgis code year-data` | 가용 기준연도 목록 |
| | `sgis code industrycode` | 산업분류 코드 |
| **config** | `sgis config set-credential` | 자격증명 설정 |
| | `sgis config get` | 설정 확인 |
| **update** | `sgis update` | 자동 업데이트 |

---

## 출력 형식

```bash
sgis data population --adm-cd 11 --year 2020 --format table    # 터미널 테이블 (기본)
sgis data population --adm-cd 11 --year 2020 --format json     # JSON
sgis data population --adm-cd 11 --year 2020 --format csv      # CSV
sgis data population --adm-cd 11 --year 2020 --format xlsx -o data.xlsx  # Excel
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson           # GeoJSON
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson --wgs84   # WGS84 재투영(Leaflet 등)
```

---

## 공통 옵션

모든 조회 명령에서 사용 가능:

- `--param key=value` — 도움말에 정의되지 않은 SGIS API 파라미터를 직접 전달(복수 지정 가능).
  예: `sgis geocode rgeocode --x-coor 953932 --y-coor 1952053 --param addr_type=21`
- `--format table|json|csv|geojson|xlsx`, `-o <파일>` — 출력 형식 및 파일 저장

**boundary 전용:**
- `--wgs84` — 좌표를 UTM-K(EPSG:5179)에서 WGS84(EPSG:4326)로 재투영. Leaflet 등 웹지도에 바로 사용 가능.

---

## MCP 서버

`sgis-mcp` 는 SGIS 조회를 **읽기 전용 MCP 도구**로 내놓는 stdio MCP 서버입니다(별도 실행 파일, `sgis` CLI 동작은 그대로).
CLI 와 같은 API 클라이언트·엔드포인트 레지스트리·토큰 발급·좌표 재투영 코드를 함수로 직접 씁니다(CLI 프로세스를 띄우지 않음).
MCP over stdio(줄 단위 JSON-RPC 2.0, protocolVersion `2025-06-18`), `initialize` · `tools/list` · `tools/call` · `ping` 지원, `tools/call` 은 동시에 처리합니다.
`--http` 를 주면 게이트웨이 없이 **Streamable HTTP** 단독 서버로 뜹니다(아래). 두 모드는 같은 도구 코드를 씁니다.

### 설치 (릴리스 자산)

CLI 와 같은 릴리스·같은 태그에 `sgis-mcp-<os>-<arch>` 가 올라가고 `SHA256SUMS` 에 함께 들어 있습니다.

```bash
VERSION=v0.5.0   # 원하는 릴리스 태그
ASSET=sgis-mcp-darwin-arm64   # sgis-mcp-{darwin,linux}-{arm64,amd64}, sgis-mcp-windows-amd64.exe
curl -fsSLO "https://github.com/clazic/sgis/releases/download/$VERSION/$ASSET"
curl -fsSLO "https://github.com/clazic/sgis/releases/download/$VERSION/SHA256SUMS"
grep " $ASSET\$" SHA256SUMS | shasum -a 256 -c -
install -m 755 "$ASSET" ~/.local/bin/sgis-mcp
sgis-mcp --version
```

소스에서: `cd src && make native` → `src/bin/sgis-mcp-<os>-<arch>`.

### 환경변수 · 키

| 이름 | 뜻 |
|------|----|
| `SGIS_API_KEYS` | 기본 키. `consumer_key:consumer_secret` 를 쉼표로 여러 개(키 풀: 라운드로빈, 429·인증 오류 키는 잠시 쉼) |
| (fallback) | `SGIS_API_KEYS` 가 없으면 `sgis` CLI 설정(`SGIS_CONSUMER_KEY`/`SGIS_CONSUMER_SECRET` → `~/.sgis/config.yaml`) 1개 |

- **요청별 키**: `tools/call` 의 `params._meta["kr.pes/keys"]` = `["consumer_key:consumer_secret", ...]`. 있으면 **그 키만** 쓰고 기본 키로 넘어가지 않습니다. 둘 다 없으면 `isError: true` 로 "API 키가 없어요".
- accessToken 은 **키 쌍별 메모리 캐시**에만 두고(4시간, 만료 24분 전 재발급) 다른 키 요청에 쓰지 않습니다. MCP 서버는 `~/.sgis/token.json` 을 읽거나 쓰지 않고, 응답 파일 캐시도 두지 않습니다.
- 키·시크릿·accessToken 은 argv·로그·결과·오류 어디에도 원문으로 나오지 않습니다(응답 직전 `***<끝4자리>` 로 스크럽, URL 인코딩 형태 포함).

### Codex / Claude Code 설정 예 (stdio)

Claude Code:
```bash
claude mcp add sgis --env SGIS_API_KEYS="KEY1:SECRET1,KEY2:SECRET2" -- ~/.local/bin/sgis-mcp
```
또는 `.mcp.json`:
```json
{ "mcpServers": { "sgis": { "command": "/Users/me/.local/bin/sgis-mcp", "env": { "SGIS_API_KEYS": "KEY1:SECRET1" } } } }
```

Codex (`~/.codex/config.toml`):
```toml
[mcp_servers.sgis]
command = "/Users/me/.local/bin/sgis-mcp"
env = { SGIS_API_KEYS = "KEY1:SECRET1" }
```

### HTTP 모드 (단독 Streamable HTTP)

```bash
sgis-mcp --http                    # 127.0.0.1:8080, 경로 /mcp
sgis-mcp --http 127.0.0.1:9100     # 주소 지정
SGIS_MCP_TOKEN=$(openssl rand -hex 32) sgis-mcp --http 0.0.0.0:9100   # loopback 밖은 토큰 필수
```

| 환경변수 | 뜻 |
|----------|----|
| `SGIS_MCP_TOKEN` | 있으면 모든 요청에 `Authorization: Bearer <토큰>` 필수(상수 시간 비교). loopback(127.0.0.1·::1·localhost)이 아닌 주소는 이 값이 없으면 **시작 거부** |
| `SGIS_MCP_ALLOWED_ORIGINS` | 쉼표 구분. `Origin` 머리글이 오면 loopback 출처(`http://localhost:*`, `http://127.0.0.1:*`)나 이 목록에 있어야 함(아니면 403) |
| `SGIS_API_KEYS` | stdio 와 같음(기본 키) |
| `MCP_RESULTS_DIR` · `MCP_RESULTS_TTL` · `MCP_RESULTS_MAX_BYTES` | 큰 결과 저장소(아래 "큰 결과"). stdio 에도 적용 |

- 규격: MCP 2025-06-18 Streamable HTTP, 직접 구현(공식 SDK 미사용 — 의존성 추가 없이 stdio 와 같은 dispatch 를 씀).
  - `POST /mcp`: JSON-RPC 하나 또는 배치(10개까지). `Accept` 에 `text/event-stream` 이 있고 `_meta.progressToken` 이 있는 `tools/call` 이면 SSE 로 답함(진행 알림들 → 마지막에 응답). 아니면 `application/json`. 알림만 온 POST 는 202.
  - 세션: `initialize` 응답 머리글 `Mcp-Session-Id`(256bit 무작위). 이후 요청은 그 머리글 필수(없으면 400, 모르는/만료 세션 404). `DELETE /mcp` 로 끝, 1시간 미사용 시 버림.
  - `GET /mcp`(세션 머리글 + `Accept: text/event-stream`): 서버 먼저 알림용 SSE, 보낼 게 없으면 15초마다 `: ping`.
  - `MCP-Protocol-Version` 머리글이 모르는 값이면 400. 본문 1MB 상한(413).
- 요청별 키: 머리글 `X-Api-Keys: key1:secret1,key2:secret2` → 그 POST 의 모든 `tools/call` 에 `_meta["kr.pes/keys"]` 와 똑같이 적용(기본 키로 안 넘어감). 둘 다 있으면 `_meta` 우선.
- HTTP 응답·SSE 이벤트 전부 키·시크릿·토큰 스크럽. 접근 기록(stderr)은 메서드·도구 이름·걸린 시간만.

Claude Code (HTTP):
```bash
claude mcp add --transport http sgis http://127.0.0.1:8080/mcp --header "Authorization: Bearer $SGIS_MCP_TOKEN"
```

Codex (`~/.codex/config.toml`, HTTP):
```toml
[mcp_servers.sgis]
url = "http://127.0.0.1:8080/mcp"
bearer_token_env_var = "SGIS_MCP_TOKEN"
```

### 진행 알림 · 취소

- `tools/call` 의 `params._meta.progressToken` 이 있으면 `notifications/progress` 를 보냅니다(stdio 는 stdout 줄, HTTP 는 그 POST 의 SSE). 시작·끝은 항상, 중간(토큰 발급, 다음 키로 넘어감, `sgis_boundary` GeoJSON 정리)은 1초에 한 번까지. 토큰이 없으면 보내지 않습니다.
- `notifications/cancelled {requestId}` 를 받으면 그 요청의 SGIS HTTP 호출을 끊고 그 요청에는 답하지 않습니다. HTTP 모드에서 POST 연결이 끊겨도 취소로 봅니다.

### 도구 (모두 읽기 전용)

| 도구 | 입력 | 하는 일 |
|------|------|---------|
| `sgis_admin_codes` | `cd?`(2·5자리) | 행정구역 코드 단계별 조회(생략=시도, 2자리=시군구, 5자리=읍면동) |
| `sgis_years` | — | 통계 가용 기준연도 |
| `sgis_population` | `year`, `adm_cd?`, `low_search?` | 총인구·평균나이·노령화지수 등 |
| `sgis_search_population` | `year`, `adm_cd?`, `low_search?`, `gender?`, `age_type?`, `edu_level?`, `mrg_state?` | 조건별 인구 |
| `sgis_household` | `year`, `adm_cd?`, `low_search?`, `household_type?`, `ocptn_type?` | 가구통계 |
| `sgis_house` | `year`, `adm_cd?`, `low_search?`, `house_type?`, `const_year?`, `house_area_cd?` | 주택통계 |
| `sgis_company` | `year`, `adm_cd?`, `low_search?`, `class_code?`, `theme_cd?` | 사업체·종사자 수 |
| `sgis_farm_household` | `year`(5년 주기), `adm_cd?`, `low_search?` | 농가통계 |
| `sgis_industry_codes` | `class_deg`, `class_code?` | KSIC 산업분류 코드 |
| `sgis_geocode` | `address`, `crs?`(wgs84·utmk), `pagenum?`, `resultcount?` | 주소→좌표 |
| `sgis_reverse_geocode` | `x`, `y`, `crs?`(wgs84·utmk) | 좌표→주소 |
| `sgis_transform_coord` | `src`, `dst`(EPSG), `x`, `y` | 좌표계 변환 |
| `sgis_boundary` | `year`, `adm_cd`, `low_search?`, `wgs84?`(기본 true), `decimals?`(0~8), `properties?`(속성 목록), `format?`(geojson), `save?` | 행정구역 경계 GeoJSON 텍스트(UTM-K→WGS84 재투영은 CLI `--wgs84` 와 같음) |
| `sgis_result_page` | `cursor`, `limit?`(행 수) | 저장된 큰 결과의 다음 쪽(같은 형식, 재조회 없음, **키 불필요**) |
| `sgis_result_query` | `result_id`, `filter?`, `aggregate?`, `format?`, `save?` | 저장된 표 결과를 거르기·집계(재조회 없음, **키 불필요**) |

통계 도구(`sgis_population` · `sgis_search_population` · `sgis_household` · `sgis_house` · `sgis_company` · `sgis_farm_household`)는 공통 선택 인자 `format?`(json·columns·csv), `filter?`, `aggregate?`, `save?` 를 받습니다.

- `adm_cd` 는 2(시도)·5(시군구)·8(읍면동)자리만, `year` 는 `YYYY`. 통계 도구에서 `adm_cd`·`low_search` 를 둘 다 생략하면 `low_search=1`(전국 시도별)로 부릅니다(둘 다 없으면 SGIS 가 빈 결과를 줌).
- 결과는 JSON 텍스트. 큰 결과는 아래 "큰 결과" 규칙대로 쪽을 나눕니다(그 밖의 작은 조회 도구는 약 200KB 에서 잘라 "잘림: 범위를 좁히세요").

### 큰 결과 (압축 형식 · 쪽 나누기 · 집계 · 전체 파일)

응답 한 번은 약 200KB 상한 그대로이고, 같은 데이터를 더 촘촘히 · 나눠서 · 줄여서 · 맥락 밖 파일로 받습니다.

- **형식** (`format`):
  - 생략: 작은 결과는 지금과 같은 원본 JSON 배열(호환), 큰 결과는 `columns`.
  - `json`: `{요약…, "data":[{…},…]}`.
  - `columns`: `{요약…, "columns":[…], "rows":[[…],…], "dict":{"adm_cd":{"11010":"서울특별시 종로구",…}}, "dict_columns":{"adm_cd":"adm_nm"}}` — 이름 열(`adm_nm`, `addr_name`)은 빼고 코드→이름을 `dict` 로 한 번만.
  - `csv`: 첫 줄 요약 JSON, 둘째 줄부터 머리줄 + 행.
  - `sgis_boundary`: GeoJSON FeatureCollection(요약 칸이 앞에 붙은 유효한 GeoJSON). `decimals` 로 좌표 자릿수, `properties` 로 속성 고르기.
- **요약 칸**(맨 앞): `total_rows`, `returned_rows`, `truncated`, `format`, `result_id`(저장했을 때), `next_cursor`(더 있을 때), `download`(저장했을 때 `{uri,name,mimeType,size,path,rows}`), 집계 시 `skipped_non_numeric`.
- **쪽 나누기**: 상한을 넘으면 결과를 저장하고 첫 쪽 + `next_cursor` 를 줍니다. `sgis_result_page {cursor, limit?}` 로 다음 쪽(다시 조회 안 함, 키 불필요). 경계는 feature 단위로 나눠 쪽마다 유효한 GeoJSON. 만료되면 isError "결과가 만료됐어요 — 다시 조회하세요".
- **거르기 · 집계**: `filter` = `{"열":["값",…]}` 목록 · `{"열":{"from":x,"to":y}}` 범위(숫자면 숫자 비교) · `{"열":"값"}`. `aggregate` = `{"group_by":["열",…],"op":"sum|avg|min|max|count","value":"숫자 열"}`(숫자 아닌 값은 건너뛰고 `skipped_non_numeric` 로 알림). 저장된 결과에는 `sgis_result_query`. SGIS 통계는 한 호출이 한 연도라 `latest` · `pivot` 은 두지 않았습니다.
- **전체 파일**: 저장하면 content 에 `resource_link`(`sgis://results/<id>.csv`, `text/csv` / `.geojson`, `application/geo+json`, `size`, `description: "전체 N행"`)가 붙습니다.
  - `resources/read {uri}` 로 파일 내용(20MB 넘으면 오류 — 내려받기 사용). `initialize` capabilities 에 `resources`. `resources/list` 는 빈 목록(결과는 임시).
  - HTTP 모드: `GET /results/<id>.csv|geojson` — `SGIS_MCP_TOKEN` 이 있으면 같은 Bearer 필요, Content-Type · Content-Length · Content-Disposition 포함.
- **저장소** (`MCP_RESULTS_DIR`, 기본 `~/.sgis/cache/mcp-results`): `<id>.jsonl`(쪽 나누기용 한 줄 한 행/feature), `<id>.csv` 또는 `<id>.geojson`, `<id>.meta.json`{id, tool, created, rows, columns, bytes, formats, args_summary, kind}. id 는 144bit 무작위(base64url). 키·토큰은 넣지 않고(받은 응답에서도 지움), 쓰기는 임시 파일 → rename(여러 프로세스 공유 가능).
  - 결과가 상한을 넘거나 `save: true` 일 때만 저장. 보관 `MCP_RESULTS_TTL`(기본 1h), 폴더 상한 `MCP_RESULTS_MAX_BYTES`(기본 1GB, 넘으면 오래된 것부터 삭제). 시작할 때와 10분마다 정리.
  - 게이트웨이 경유면 게이트웨이가 `MCP_RESULTS_DIR` 를 정해 넘기고 같은 폴더에서 직접 내려줍니다.
- 엑셀·파일 저장, 설정 변경, 업데이트, 차트·지도 이미지는 MCP 도구에 없습니다.

### smoke

- 키 없이: `initialize` → `tools/list` 성공(도구 15개).
- 키 있으면: 도구 `sgis_admin_codes`, 인자 `{}` → 시도 17개.

stdio:
```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sgis_admin_codes","arguments":{}}}' \
  | sgis-mcp
```

HTTP (`sgis-mcp --http` 를 띄운 뒤):
```bash
U=http://127.0.0.1:8080/mcp; H=(-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream')
SID=$(curl -s -D - -o /dev/null "${H[@]}" -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' $U | awk 'tolower($1)=="mcp-session-id:"{print $2}' | tr -d '\r')
curl -s "${H[@]}" -H "Mcp-Session-Id: $SID" -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' $U
curl -s "${H[@]}" -H "Mcp-Session-Id: $SID" -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sgis_admin_codes","arguments":{}}}' $U
```

---

## 지원 플랫폼

| OS | 아키텍처 | 설치 방법 |
|----|---------|---------|
| macOS (Apple Silicon) | arm64 | curl install.sh |
| macOS (Intel) | amd64 | curl install.sh |
| Linux | amd64 | curl install.sh |
| Linux | arm64 | curl install.sh |
| Windows 10+ | amd64 | PowerShell install.ps1 |

---

## 관련 링크

- 스킬 가이드: [SKILL.md](SKILL.md)
- 학습 로그: [LEARNINGS.md](LEARNINGS.md)
- 상세 문서: [references/](references/)
- 행정구역 코드표: [references/region-codes.md](references/region-codes.md)
- 자격증명 발급: [https://sgis.mods.go.kr/developer/](https://sgis.mods.go.kr/developer/)
- 릴리스: [GitHub Releases](https://github.com/clazic/sgis/releases)

---

## 라이선스

MIT
