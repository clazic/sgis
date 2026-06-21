# 03 — 명령어 레퍼런스

## 명령 구조

```
sgis <그룹> <서브커맨드> [파라미터...] [--format <포맷>] [-o <파일>]
```

4개 데이터 그룹 + config + update:

```
sgis
├── data        — 통계 (인구/가구/주택/사업체/농림어가 등)
├── boundary    — 경계 GeoJSON (행정구역/집계구/격자 등)
├── geocode     — 지오코딩·좌표변환
├── code        — 행정구역·산업분류 코드
├── config      — 자격증명·설정 관리
└── update      — 바이너리·스킬 자동 업데이트
```

---

## 공통 플래그

| 플래그 | Short | 기본 | 설명 |
|--------|-------|------|------|
| `--format` | `-f` | `table` | 출력 포맷: `table\|json\|csv\|geojson\|xlsx` |
| `--output` | `-o` | stdout | 출력 파일 경로 (xlsx는 필수) |
| `--help` | `-h` | | 도움말 |

---

## data — 통계 조회

### 공통 파라미터

| 플래그 | 설명 |
|--------|------|
| `--adm-cd` | 행정구역 코드 (미지정=전국, 2자리=시도, 5자리=시군구, 8자리=읍면동) |
| `--year` | 기준연도 (엔드포인트별 범위 다름) |
| `--low-search` | 0=해당코드만, 1=1단계 하위, 2=2단계 하위 |

### 서브커맨드 목록

```bash
sgis data population [--adm-cd <코드>] [--year <연도>] [--low-search 0|1|2]
    # 인구통계: tot_ppltn, avg_age, ppltn_dnsty, tot_family, tot_house 등
    # 지원연도: 2015~2024

sgis data household [--adm-cd <코드>] [--year <연도>] [--low-search 0|1|2]
    # 가구통계: household_cnt, family_member_cnt, avg_family_member_cnt
    # 지원연도: 2015~2024

sgis data house [--adm-cd <코드>] [--year <연도>] [--house-type <유형>]
    # 주택통계: house_cnt
    # 지원연도: 2015~2024

sgis data company [--adm-cd <코드>] [--year <연도>] [--class-code <코드>]
    # 사업체통계: corp_cnt, tot_worker
    # 지원연도: 2000~2024

sgis data searchpopulation [--adm-cd <코드>] [--year <연도>] [--gender M|F]
    # 인구검색(조건별): population
    # 추가 플래그: --age-type, --edu-level, --mrg-state

sgis data farmhousehold [--adm-cd <코드>] [--year <연도>]
    # 농가통계: 지원연도 2000/2005/2010/2015/2020 (5년 주기)

sgis data forestryhousehold [--adm-cd <코드>] [--year <연도>]
    # 임가통계: 지원연도 2000/2005/2010/2015/2020

sgis data fisheryhousehold [--adm-cd <코드>] [--year <연도>] [--oga-div <구분>]
    # 어가통계: 지원연도 2000/2005/2010/2015/2020

sgis data householdmember [--adm-cd <코드>] [--year <연도>] [--data-type <유형>]
    # 가구원통계: population
    # 추가 플래그: --gender, --age-from, --age-to
```

### 예시

```bash
# 서울 2020년 인구 (table 출력)
sgis data population --adm-cd 11 --year 2020

# 경기도 시군구별 사업체통계 JSON
sgis data company --adm-cd 31 --year 2023 --low-search 1 --format json

# 전국 가구통계 Excel 저장
sgis data household --year 2023 --format xlsx -o 가구통계2023.xlsx

# 전국 농가통계 (5년 주기)
sgis data farmhousehold --year 2020
```

---

## boundary — 경계 GeoJSON 조회

> 출력은 GeoJSON `FeatureCollection`. 좌표계: **UTM-K (EPSG:5179)**.
> Leaflet 등 WGS84 기반 지도에서 사용하려면 좌표변환 필요.

```bash
sgis boundary hadmarea [--adm-cd <코드>] [--year 2000~2025] [--low-search 0|1|2]
    # 행정구역 경계 — 가장 자주 사용

sgis boundary statsarea --adm-cd <8자리코드>
    # 집계구 경계 (읍면동 코드 8자리 필수)

sgis boundary userarea --minx <> --miny <> --maxx <> --maxy <> --cd 1|2|3|4
    # 영역내 경계 (UTM-K bbox 입력, cd: 1=시도/2=시군구/3=읍면동/4=집계구)

sgis boundary urbanboundary [도시권 파라미터]
    # 도시/준도시 경계

sgis boundary grid [격자 파라미터]
    # 행정구역 격자경계

sgis boundary buildingarea
    # 전개도 건물경계

sgis boundary floorboundary
    # 층별 최외각 공간속성

sgis boundary floorcompany
    # 층별 사업체 공간속성
```

### 예시

```bash
# 서울 2025년 행정구역 경계 GeoJSON 파일 저장
sgis boundary hadmarea --adm-cd 11 --year 2025 --format geojson -o seoul-2025.geojson

# 경기도 시군구 경계
sgis boundary hadmarea --adm-cd 31 --year 2025 --low-search 1 --format geojson

# UTM-K bbox로 영역 내 시군구 경계
sgis boundary userarea --minx 900000 --miny 1900000 --maxx 1000000 --maxy 2000000 --cd 2
```

---

## geocode — 지오코딩 / 좌표변환

```bash
sgis geocode address <주소문자열> [--pagenum 0] [--resultcount 1~50]
    # 주소 → 좌표 (UTM-K 출력)
    # 응답: X(UTM-K), Y(UTM-K), sido_nm, sgg_nm, adm_nm, road_nm 등

sgis geocode address-wgs84 <주소문자열>
    # 주소 → 좌표 (WGS84 출력)

sgis geocode reverse --x-coor <UTM-K X> --y-coor <UTM-K Y>
    # 좌표(UTM-K) → 주소
    # 응답: sido_nm, sgg_nm, emdong_nm, road_nm, full_addr 등

sgis geocode reverse-wgs84 --x-coor <경도> --y-coor <위도>
    # 좌표(WGS84) → 주소

sgis geocode transform --src <좌표계코드> --dst <좌표계코드> --pos-x <X> --pos-y <Y>
    # 좌표계 변환 (예: EPSG:5179 → EPSG:4326)
    # 응답: posX, posY (변환된 좌표)
```

### 예시

```bash
# 주소 → UTM-K 좌표
sgis geocode address "서울특별시 종로구 청와대로 1"

# 주소 → WGS84 좌표 (지도 앱 호환)
sgis geocode address-wgs84 "부산광역시 해운대구 우동 1413"

# UTM-K 좌표 → 주소
sgis geocode reverse --x-coor 953932 --y-coor 1952053

# UTM-K → WGS84 변환
sgis geocode transform --src 5179 --dst 4326 --pos-x 953932 --pos-y 1952053
```

---

## code — 행정구역 코드 조회

```bash
sgis code stage [--cd <코드>] [--pg-yn 0|1]
    # 단계별 주소 조회
    # cd 미지정=시도 17개, 2자리=시군구, 5자리=읍면동
    # pg_yn: 경량화 경계 포함 여부

sgis code findcodeinsmallarea [파라미터]
    # 소지역(집계구) 코드 찾기

sgis code year-data
    # 통계 가용 기준연도 목록

sgis code industrycode --class-deg <차수>
    # 산업분류 코드 (class_deg: 2000~05=8, 06~16=9, 17~23=10, 24~=11)
```

### 예시

```bash
sgis code stage                    # 시도 17개
sgis code stage --cd 11            # 서울 구 25개
sgis code stage --cd 11010         # 종로구 읍면동 목록
sgis code year-data                # 가용 연도 목록
sgis code industrycode --class-deg 10   # 10차 산업분류
```

---

## config — 설정 관리

```bash
sgis config set-credential <CONSUMER_KEY> <CONSUMER_SECRET>
    # consumerKey + consumerSecret 등록 (config.yaml에 저장, 0o600)

sgis config get
    # 현재 설정 표시 (secret 마스킹)
```

---

## update — 자동 업데이트

```bash
sgis update
    # 최신 버전 바이너리 + 스킬 파일 업데이트
    # GitHub releases/latest 비교 → 다운로드 → SHA256 검증 → 교체

sgis update --check
    # 업데이트 확인만 (설치 안 함)

sgis update --force
    # 현재 버전과 같아도 강제 재설치
```

---

## 출력 포맷 상세

| 포맷 | 플래그 | 파일 확장자 | 용도 |
|------|--------|------------|------|
| `table` | `-f table` | — | 터미널 확인 (기본) |
| `json` | `-f json` | `.json` | AI 파이프라인, jq 처리 |
| `csv` | `-f csv` | `.csv` | 스프레드시트 |
| `geojson` | `-f geojson` | `.geojson` | GeoJSON 공간분석 |
| `xlsx` | `-f xlsx` | `.xlsx` | Excel (`-o` 필수) |

```bash
# 파일로 저장 (확장자로 포맷 자동 감지)
sgis data population --adm-cd 11 --year 2020 -o 서울인구.xlsx
sgis boundary hadmarea --adm-cd 11 --year 2025 -o seoul.geojson

# stdout으로 JSON 출력 후 jq 처리
sgis data population --adm-cd 11 --year 2020 --format json | jq '.[] | .tot_ppltn'
```
