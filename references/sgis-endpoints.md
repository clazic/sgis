# SGIS Open API 엔드포인트 레지스트리 (canonical reviewed-scrape record)

- 출처: https://sgis.mods.go.kr/developer/html/newOpenApi/api/dataApi/basics.html
- API 호스트: `https://sgisapi.mods.go.kr`
- 스크랩/검토 일자: 2026-06-21 (Playwright MCP)
- 검증: 인증·데이터·코드 호출은 라이브 API로 실측 검증함(아래 "## 인증 (검증됨)" 참조). 시크릿은 마스킹.
- 공통 응답 envelope: `{ "result": <object|array>, "errCd": <int>, "errMsg": <string>, "id": <string>, "trId": <string>}`
  - 성공 시 `errCd: 0`, `errMsg: "Success"`. GeoJSON 엔드포인트는 envelope 없이 GeoJSON `FeatureCollection` 직접 반환.
- 공통 요청 파라미터: 거의 모든 데이터 엔드포인트는 `accessToken`(필수, 쿼리파라미터)을 요구. 인증 API 2종만 예외.
- 좌표계(CRS): 통계/주소/경계 좌표 기본은 **UTM-K (EPSG:5179)** — 응답에 `UTM-K`로 명시됨. WGS84(EPSG:4326)는 `geocodewgs84`/`rgeocodewgs84` 전용 엔드포인트에서만 제공. 경계(.geojson) 좌표도 UTM-K. (CRS 상세는 맨 아래 "## 좌표계(CRS) 정리" 참조)

---

## 그룹 매핑 요약 (CLI 4그룹 ↔ API)

| CLI 그룹 | 대응 API path prefix | 비고 |
|---|---|---|
| `data` (통계) | `/OpenAPI3/stats/*`, `/themamap/*`, `/jibang/*`, `/urban/*`, `/startupbiz/*`, `/technicalbiz/*`, `/lastname/*`, `/ndsm/*`, `/statscommunity/*` | 인구/가구/주택/사업체/농림어가 + 주제도·지방통계·도시권·창업·기술업종·성씨·재해 |
| `boundary` (경계) | `/OpenAPI3/boundary/*.geojson`, `/urban/boundary.geojson`, `/figure/*.geojson`, `/grid/data.geojson` | 행정구역/집계구/영역내/도시권/건물/격자 경계(GeoJSON) |
| `geocode` (지오코딩/좌표) | `/OpenAPI3/addr/geocode*.json`, `/addr/rgeocode*.json`, `/transformation/transcoord.json` | 주소→좌표, 좌표→주소, 좌표변환 |
| `code` (행정구역코드) | `/OpenAPI3/addr/stage.json`, `/personal/findcodeinsmallarea.json`, `/year/data.json`, `/stats/industrycode.json` | 시도/시군구/읍면동 코드, 소지역코드, 기준연도, 산업분류 |

전체 엔드포인트: **약 87개** (16개 문서 페이지). 그룹별 개수는 본 문서 끝 "## 엔드포인트 개수 요약" 참조.

---

## 인증 (auth)

문서 페이지: basics.html (#auth)

| API 이름 | Method | 엔드포인트 |
|---|---|---|
| 인증키 이용 인증(accessToken 발급) | GET | `/OpenAPI3/auth/authentication.json` |
| JavaScript API 인증 | GET | `/OpenAPI3/auth/javascriptAuth.json` |

### authentication.json 요청
| 요청변수 | 값 | 필수 | 설명 |
|---|---|---|---|
| `consumer_key` | String | 필수 | 서비스 ID (= config의 consumer_key) |
| `consumer_secret` | String | 필수 | 서비스 Secret |

### authentication.json 응답 (`result` 내부)
| 출력변수 | 값 | 설명 |
|---|---|---|
| `accessToken` | String | OAuth 인증키. 이후 API 호출 시 `accessToken` 쿼리파라미터로 전달 |
| `accessTimeout` | String | 문서상 "1970-01-01 0시부터 현재까지의 초". **실측 결과 13자리 밀리초 epoch(만료 절대시각)** — 아래 검증 섹션 참조 |

### javascriptAuth.json 요청
| 요청변수 | 값 | 필수 | 설명 |
|---|---|---|---|
| `consumer_key` | String | 필수 | 서비스 ID |
응답: `Javascript API`(자바스크립트 본문 전송). CLI에서는 사용하지 않음.

---

## 인증 (검증됨) — 2026-06-21 라이브 실측

> credential은 `~/.sgis/config.yaml`에서 로드. consumer_key/secret 모두 20자, 마스킹: `f52b****` / `ba79****`.

**발급 요청 (실측):**
```
GET https://sgisapi.mods.go.kr/OpenAPI3/auth/authentication.json?consumer_key=f52b****&consumer_secret=ba79****
```
- 파라미터명은 **`consumer_key`, `consumer_secret`** (snake_case, 쿼리스트링). Body가 아닌 쿼리로 동작 확인.

**발급 응답 (실측, 토큰 마스킹):**
```json
{
  "result": { "accessToken": "564430...(MASKED)", "accessTimeout": "1782022524954" },
  "errCd": 0,
  "errMsg": "Success",
  "id": "API_0101",
  "trId": "QdDK_API_0101_..."
}
```

**확정 사실(CONFIRMED):**
- 발급 URL: `https://sgisapi.mods.go.kr/OpenAPI3/auth/authentication.json`
- 요청 파라미터명: `consumer_key`, `consumer_secret` (쿼리)
- 응답 필드 경로: `result.accessToken`, `result.accessTimeout`
- 상태 필드: `errCd`(int, 성공 0), `errMsg`("Success"), `id`, `trId`
- **`accessTimeout` 단위 = 밀리초 단위 Unix epoch(만료 절대시각)**. 문서의 "초" 설명은 틀림. 값 `1782022524954`는 13자리 ms.
- **토큰 유효기간 = 발급시점 기준 약 4시간**. 실측: `accessTimeout - now = 14,387,015 ms ≈ 14400초 = 4.0시간`. (즉 SGIS accessToken 수명은 **4시간(14,400,000 ms)**.)
- **auth.go 구현 주의:** `expiresAt`은 `accessTimeout`을 ms로 파싱(`/1000` 하여 Unix초 또는 그대로 `time.UnixMilli`). 적응형 재발급 임계 `max(60s, 0.1*lifetime)`에서 lifetime=4h이면 임계 ≈ 24분 전 재발급.

**데이터 호출에서 토큰 전달 방식 (실측):**
```
GET .../OpenAPI3/stats/population.json?accessToken=<TOKEN>&year=2020&adm_cd=11&low_search=0
```
- 토큰은 **쿼리파라미터 `accessToken`** 으로 전달(헤더 아님).
- 응답 envelope 동일: `{result:[...], errCd:0, errMsg:"Success", id:"API_0301", trId:...}`.
- 서울(adm_cd=11, 2020) 인구 호출 성공: `tot_ppltn=9586195`, `tot_family=3982290` 등 1행 반환.

**코드/지역 호출 (실측):** `GET .../OpenAPI3/addr/stage.json?accessToken=<TOKEN>` →
`result:[{cd:"11", addr_name:"서울특별시", full_addr:"서울특별시", x_coor:"953932", y_coor:"1952053"}, {cd:"21","부산광역시"...}, ...]` (시도 17개 목록). 좌표 x≈95만/y≈195만 = **UTM-K 미터(EPSG:5179)** 확정.

**에러 처리 메모(auth 패키지용):** 성공은 `errCd==0`로 판정. 잘못된 credential이면 `errCd != 0` + `errMsg`로 거부 신호가 올 것으로 예상(라이브에서 의도적 오류 호출은 미수행 — rate 보호). `ErrInvalidCredential`는 `errCd != 0` AND HTTP 200 케이스로, `ErrNetwork`는 전송 실패로 분기 권장.

---

## data — 통계 (`/OpenAPI3/stats/*`) — census.html

| API 이름 | Method | 엔드포인트 | 필수 파라미터 | 주요 선택 파라미터 |
|---|---|---|---|---|
| 인구통계 | GET | `/OpenAPI3/stats/population.json` | accessToken, year | adm_cd, low_search |
| 인구검색(검색조건) | GET | `/OpenAPI3/stats/searchpopulation.json` | accessToken, year | gender, adm_cd, low_search, age_type, edu_level, mrg_state |
| 가구통계 | GET | `/OpenAPI3/stats/household.json` | accessToken, year | adm_cd, low_search, household_type, ocptn_type |
| 주택통계 | GET | `/OpenAPI3/stats/house.json` | accessToken, year | adm_cd, low_search, house_type, const_year, house_area_cd |
| 사업체통계 | GET | `/OpenAPI3/stats/company.json` | accessToken, year | adm_cd, low_search, class_code, theme_cd |
| 산업분류 | GET | `/OpenAPI3/stats/industrycode.json` | accessToken, class_deg | class_code |
| 농가통계 | GET | `/OpenAPI3/stats/farmhousehold.json` | accessToken, year | adm_cd, low_search |
| 임가통계 | GET | `/OpenAPI3/stats/forestryhousehold.json` | accessToken, year | adm_cd, low_search |
| 어가통계 | GET | `/OpenAPI3/stats/fisheryhousehold.json` | accessToken, year, oga_div | adm_cd, low_search |
| 가구원통계 | GET | `/OpenAPI3/stats/householdmember.json` | accessToken, year, data_type | adm_cd, low_search, gender, age_from, age_to |

**공통 파라미터 값 규칙:**
- `year`: 인구/주택 2015~2024, 사업체 2000~2024, 농림어가 2000/2005/2010/2015/2020 (엔드포인트별 상이).
- `adm_cd`: 미지정=전국 시도리스트, 2자리=시도, 5자리=시군구, 8자리=읍면동.
- `low_search`: 0=해당코드만, 1=1단계 하위, 2=2단계 하위 (default 보통 1; 농림어가는 default 0).

**population.json 응답 주요 필드:** `adm_cd, adm_nm, tot_ppltn(총인구), avg_age(평균나이), ppltn_dnsty(인구밀도), aged_child_idx(노령화지수), oldage_suprt_per, juv_suprt_per, tot_family(총가구), avg_fmember_cnt, tot_house(총주택), nongga_cnt, imga_cnt, naesuoga_cnt, haesuoga_cnt, employee_cnt, corp_cnt` 등.
**household.json:** `adm_cd, adm_nm, household_cnt, family_member_cnt, avg_family_member_cnt`.
**house.json:** `adm_cd, adm_nm, house_cnt`.
**company.json:** `adm_cd, adm_nm, corp_cnt, tot_worker`.
**searchpopulation.json:** `adm_cd, adm_nm, population`.
**industrycode.json:** `class_code, class_nm` (class_deg: 2000~05=8, 06~16=9, 17~23=10, 24~=11).
**farm/forestry/fisheryhousehold.json:** `adm_cd, adm_nm, *_cnt, population, avg_population`.
**householdmember.json:** `adm_cd, adm_nm, population`.

샘플(실측, 서울 2020 인구): `tot_ppltn=9586195, tot_family=3982290, avg_age=42.4, ppltn_dnsty=15837.1`.

---

## search — 검색 (`/OpenAPI3/search/*`) — search.html

| API 이름 | Method | 엔드포인트 | 필수 파라미터 | 주요 선택 파라미터 |
|---|---|---|---|---|
| 연관어검색 | GET | `/OpenAPI3/search/relword.json` | accessToken, searchword | — |
| SOP검색 | GET | `/OpenAPI3/search/sop.json` | accessToken, searchword | pagenum(default 0), resultcount(1~50, default 5) |

**relword.json 응답:** `rel_search_word`(유의어 리스트). (API_0501)
**sop.json 응답:** `totalcount, pagenum, returncount, resultData[]{stat_id, data_base_year, nm, url}`. (API_0502)

> SGIS 공식 메뉴에서 '검색'은 센서스와 별개 그룹이며 하위에 연관어검색·SOP검색 2개가 있다.
> CLI에서는 `sgis search relword` / `sgis search sop` 로 제공한다.

---

## data — 주제도/지방/도시권/창업/기술업종/성씨/재해 (census 외 통계 페이지)

### 총조사 주요지표 묶음은 census.html (위 stats/* 표가 정본).

### 인구와 가구 등 주제도 (thematicMapCTGR.html) `/OpenAPI3/themamap/*`
| API | 엔드포인트 |
|---|---|
| 인구와 가구 목록/상세 | `/themamap/CTGR_001/list.json` · `/themamap/CTGR_001/data.json` |
| 주거와 교통 목록/상세 | `/themamap/CTGR_002/list.json` · `/themamap/CTGR_002/data.json` |
| 복지와 문화 목록/상세 | `/themamap/CTGR_003/list.json` · `/themamap/CTGR_003/data.json` |
| 노동과 경제 목록/상세 | `/themamap/CTGR_004/list.json` · `/themamap/CTGR_004/data.json` |

### 가구/인구비율 (jibang.html) `/OpenAPI3/jibang/*`
| API | 엔드포인트 |
|---|---|
| 가구/인구비율 목록/상세 | `/jibang/category_a/list.json` · `/jibang/category_a/data.json` |
| 사회비율 목록/상세 | `/jibang/category_b/list.json` · `/jibang/category_b/data.json` |
| 주택비율 목록/상세 | `/jibang/category_c/list.json` · `/jibang/category_c/data.json` |
| 교통비율 목록/상세 | `/jibang/category_d/list.json` · `/jibang/category_d/data.json` |
| 종교비율 목록/상세 | `/jibang/category_e/list.json` · `/jibang/category_e/data.json` |
| 사업체비율 목록/상세 | `/jibang/category_f/list.json` · `/jibang/category_f/data.json` |

### 도시권 (urban.html) `/OpenAPI3/urban/*`
| API | 엔드포인트 | 그룹 |
|---|---|---|
| 도시권 목록(카테고리) | `/urban/category.json` | data |
| 도시/준도시 목록 | `/urban/list.json` | data |
| 도시/준도시 경계 | `/urban/boundary.geojson` | **boundary** |
| 도시별 인구 통계 | `/urban/ingu/data.json` | data |
| 도시별 가구 통계 | `/urban/gagu/data.json` | data |
| 도시별 주택 통계 | `/urban/ho/data.json` | data |
| 사업체 통계 | `/urban/corp/data.json` | data |
| 주요지표 통계 | `/urban/to/data.json` | data |
| 도시별 생활시설 통계 | `/urban/fac/data.json` | data |

### 생활업종 후보지검색 (lifeBizStatsMap.html) `/OpenAPI3/startupbiz/*`
| API | 엔드포인트 |
|---|---|
| 생활업종 후보지검색 | `/startupbiz/startupbiz.json` |
| 거주인구 요약 | `/startupbiz/pplsummary.json` |
| 성별인구비율 요약 | `/startupbiz/mfratiosummary.json` |
| 거처종류 요약 | `/startupbiz/housesummary.json` |
| 소상공인 업종별 사업체비율 | `/startupbiz/corpdistsummary.json` |
| 소상공인 업종별 사업체증감 | `/startupbiz/corpindecrease.json` |
| 생활업종 후보지 정보 | `/startupbiz/regiontotal.json` |
| 시도별 생활업종 순위 | `/startupbiz/sidotobrank.json` |
| 시도별 생활업종 정보 | `/startupbiz/sidotobinfo.json` |
| 시도별 생활업종 속성 | `/startupbiz/sidotobgroup.json` |
| 시군구별 생활업종 사업체수 | `/startupbiz/sggtobcorpcount.json` |
| 시군구별 생활업종 정보 | `/startupbiz/sggtobinfo.json` |
| 생활업종별 시군구 순위 | `/startupbiz/sggtobrank.json` |
| 생활업종 후보지 비교 | `/startupbiz/Compareregiontotal.json` |

### 전국 기술업종 정보 (techBizStatsMap.html) `/OpenAPI3/technicalbiz/*`
| API | 엔드포인트 |
|---|---|
| 전국 기술업종 정보 | `/technicalbiz/companyinfo.json` |
| 시도별 기술업종 정보 | `/technicalbiz/sidocompanyinfo.json` |
| 시군구별 기술업종 정보 | `/technicalbiz/sggcompanyinfo.json` |

### 50대 성씨 (lastName.html) `/OpenAPI3/lastname/*`
| API | 엔드포인트 |
|---|---|
| 50대 성씨 목록 | `/lastname/list.json` |
| 50대 성씨 시군구별 인구수 | `/lastname/data.json` |

### 지역현안 소통지도 (community.html)
| API | 엔드포인트 |
|---|---|
| 지역현안 소통지도 목록 | `/statscommunity/list.json` |

### 자연재해 (naturalCalamity.html) `/OpenAPI3/ndsm/*`
| API | 엔드포인트 |
|---|---|
| 년도별 태풍정보 목록 | `/ndsm/typInfoYearList.json` |
| 태풍별 영향범위 목록 | `/ndsm/typInfoAdmCdList.json` |
| 태풍별 통계정보 상세 | `/ndsm/typDataBoard.json` |
| 홍수위험지도 영향범위 목록 | `/ndsm/floodRiskAdmCdList.json` |
| 홍수위험지도 통계정보 상세 | `/ndsm/floodRiskDataBoard.json` |
| 산사태위험지도 영향범위 목록 | `/ndsm/lndsldWarnAdmCdList.json` |
| 산사태위험지도 통계정보 상세 | `/ndsm/lndsldWarnDataBoard.json` |
| 과거 폭염특보 목록 | `/ndsm/prevHwSpcnwsList.json` |
| 과거 폭염 영향예보 목록 | `/ndsm/prevHwImpctFrcstList.json` |
| 폭염 통계정보 상세 | `/ndsm/hwDataBoard.json` |

---

## boundary — 경계 (GeoJSON) — addressBoundary.html / 외

> 출력은 GeoJSON `FeatureCollection`: `features[].{type, geometry:{type, coordinates}, properties:{...}}`. **좌표계 UTM-K(EPSG:5179)** (별도 표기 없음, bbox 입력이 UTM-K이며 좌표값이 미터 단위).

| API 이름 | Method | 엔드포인트 | 필수 파라미터 |
|---|---|---|---|
| 행정구역경계 | GET | `/OpenAPI3/boundary/hadmarea.geojson` | accessToken, year(2000~2025), adm_cd; (선택 low_search) |
| 집계구경계 | GET | `/OpenAPI3/boundary/statsarea.geojson` | accessToken, adm_cd(8자리) |
| 영역내경계 | GET | `/OpenAPI3/boundary/userarea.geojson` | accessToken, minx, miny, maxx, maxy (UTM-K), cd(1시도/2시군구/3읍면동/4집계구) |
| 도시/준도시 경계 | GET | `/OpenAPI3/urban/boundary.geojson` | accessToken (+도시권 파라미터) |
| 행정구역 격자경계 | GET | `/OpenAPI3/grid/data.geojson` | accessToken (+격자 파라미터) |
| 전개도 건물경계 | GET | `/OpenAPI3/figure/buildingarea.geojson` | accessToken |
| 층별 최외각 공간속성 | GET | `/OpenAPI3/figure/floorboundary.geojson` | accessToken |
| 층별 사업체 공간속성 | GET | `/OpenAPI3/figure/floorcompany.geojson` | accessToken |

**hadmarea 응답 properties:** `adm_cd, adm_nm, addr_en(최신연도만), x, y`. **statsarea properties:** `base_year, sido_cd/nm, sgg_cd/nm, emdong_cd, adm_cd, adm_nm, addr_en, x, y`.

### 전개도 건물정보 (deploymentChart.html) — json 형
| API | 엔드포인트 | 그룹 |
|---|---|---|
| 건물 정보 | `/figure/buildingattribute.json` | data |
| 층별 시설물 공간속성 | `/figure/flooretcfacility2.json` | data |
| 건물층별 사업체정보 | `/figure/floorcompanyinfo.json` | data |

---

## geocode — 지오코딩/좌표변환 — addressBoundary.html / coord.html

| API 이름 | Method | 엔드포인트 | 필수 파라미터 | 좌표계 |
|---|---|---|---|---|
| 지오코딩 (주소→좌표) | GET | `/OpenAPI3/addr/geocode.json` | accessToken, address | 출력 X/Y = **UTM-K** |
| 지오코딩(WGS84) | GET | `/OpenAPI3/addr/geocodewgs84.json` | accessToken, address | 출력 X/Y = **WGS84** |
| 리버스 지오코딩 (좌표→주소) | GET | `/OpenAPI3/addr/rgeocode.json` | accessToken, x_coor, y_coor (UTM-K) | 입력 UTM-K |
| 리버스 지오코딩(WGS84) | GET | `/OpenAPI3/addr/rgeocodewgs84.json` | accessToken, x_coor, y_coor (WGS84) | 입력 WGS84 |
| 좌표변환 | GET | `/OpenAPI3/transformation/transcoord.json` | accessToken, src, dst, posX, posY | src/dst=좌표계코드표 |

**geocode 선택 파라미터:** `pagenum`(default 0), `resultcount`(1~50, default 5).
**geocode 응답:** `totalcount, pagenum, returncount, matching(0완전/1불완전), resultdata[]{sido_nm/cd, sgg_nm/cd, adm_nm/cd, leg_nm/cd, ri_nm/cd, road_nm/cd, road_nm_main_no, road_nm_sub_no, bd_main_nm, bd_sub_nm, jibun_main_no, jibun_sub_no, X, Y, addr_type}`.
**rgeocode 응답:** `sido_nm/cd, sgg_nm/cd, emdong_nm/cd, main_no, sub_no, adm_dr_cd, road_nm/cd, bd_nm, sub_bd_nm, road_nm_main_no/sub_no, addr_en, full_addr`. `addr_type`: 10=도로명, 20=행정동(읍면동), 21=행정동(지번함).
**transcoord 응답:** `posX, posY` (변환된 좌표). src/dst는 좌표계코드표(EPSG 등) 참고.

---

## code — 행정구역코드/기준연도 — addressBoundary.html / personal.html

| API 이름 | Method | 엔드포인트 | 필수 파라미터 | 비고 |
|---|---|---|---|---|
| 단계별 주소 조회(시도/시군구/읍면동 코드) | GET | `/OpenAPI3/addr/stage.json` | accessToken | cd(미지정=시도, 2자리=시군구, 5자리=읍면동), pg_yn(0/1) |
| 소지역 코드찾기 | GET | `/OpenAPI3/personal/findcodeinsmallarea.json` | accessToken | 소지역(집계구) 코드 검색 |
| 최신/전체 기준년도 조회 | GET | `/OpenAPI3/year/data.json` | accessToken | 통계 가용 연도 목록 |
| 산업분류 코드 | GET | `/OpenAPI3/stats/industrycode.json` | accessToken, class_deg | (data 그룹과 공유) |

**stage.json 응답:** `cd, addr_name, full_addr, x_coor(UTM-K), y_coor(UTM-K), pg(경량화 경계)`.
**stage.json 실측:** 시도 17개 — `{cd:"11",addr_name:"서울특별시"}, {cd:"21","부산광역시"}, {cd:"22","대구광역시"}, {cd:"23","인천광역시"}, {cd:"24","광주광역시"}, {cd:"25","대전광역시"}, ...`. → `code sido` 명령의 1차 source-of-truth로 references/ 에 스냅샷 가능.

---

## 좌표계(CRS) 정리 (검증됨)

- **기본 좌표계 = UTM-K (EPSG:5179, Korea 2000 / Unified CS)**. 통계 stage 좌표(`x_coor`,`y_coor`), boundary geojson 좌표, geocode/rgeocode 기본형, userarea bbox 입력이 모두 UTM-K(미터). 실측 서울 시도좌표 x≈953,932 / y≈1,952,053 (UTM-K 미터 범위와 일치).
- **WGS84(EPSG:4326)는 전용 엔드포인트에서만**: `addr/geocodewgs84.json`, `addr/rgeocodewgs84.json`. 일반 경계(.geojson)는 WGS84 미제공.
- **결론(G2 해소):** SGIS 경계는 **WGS84가 아님 → EPSG:5179**. Leaflet 지도(EPSG:4326 기대)에 쓰려면 boundary 좌표를 4326으로 **재투영 필요**. 단순 선형 변환 불가(datum 변환). → geojson.go/map.go 재투영 또는 좌표변환 API(`transcoord`, src=EPSG:5179 dst=EPSG:4326) 연계 필요.

---

## 엔드포인트 개수 요약

| 그룹 | 개수 | 비고 |
|---|---|---|
| auth | 2 | authentication, javascriptAuth |
| data (통계 전체) | 약 60 | stats 10 + themamap 8 + jibang 12 + urban 8(경계 제외) + startupbiz 14 + technicalbiz 3 + lastname 2 + statscommunity 1 + ndsm 10 + figure(json) 3 |
| boundary (경계 GeoJSON) | 8 | hadmarea, statsarea, userarea, urban/boundary, grid/data, figure 3종 |
| geocode (지오코딩/좌표) | 5 | geocode, geocodewgs84, rgeocode, rgeocodewgs84, transcoord |
| search (검색) | 2 | search/relword, search/sop |
| code (코드/연도) | 4 | addr/stage, personal/findcodeinsmallarea, year/data, stats/industrycode(공유) |
| **합계** | **약 81~87** | 일부 엔드포인트는 그룹 간 공유(urban=data+boundary, industrycode=data+code) |
