# SGIS 차트 · 지도 HTML 템플릿

AI가 sgis-cli 데이터를 시각화할 때 사용하는 HTML 템플릿입니다.
통계 차트(ECharts)와 공간 지도(Leaflet)를 모두 지원합니다.

---

## 템플릿 목록

| 템플릿 | 용도 | 라이브러리 |
|--------|------|-----------|
| `line-chart.html` | 시계열 추이 (단일/다중 시리즈) | ECharts |
| `bar-rank.html` | 항목별 순위 (가로 막대, 자동 정렬) | ECharts |
| `pie-share.html` | 구성비/점유율 (도넛 차트) | ECharts |
| `map.html` | 행정구역 경계 단순 표시 (GeoJSON 레이어) | Leaflet + proj4js |
| `map-choropleth.html` | 단계구분도 (속성값으로 색상 계층화) | Leaflet + proj4js |

---

## 공통 플레이스홀더

모든 템플릿에서 아래 플레이스홀더를 치환합니다.

| 플레이스홀더 | 설명 | 필수 |
|-------------|------|:---:|
| `{{TITLE}}` | 페이지/차트 제목 | O |
| `{{SUBTITLE}}` | 부제목/설명 | O |
| `{{SOURCE}}` | 출처 표기 | O |
| `{{NOTE}}` | 주석 (NOTE_START~NOTE_END 주석 해제 필요, 차트 전용) | X |

**`{{SOURCE}}` 작성 예시:**
```
출처: SGIS 「행정구역 경계(읍면동)」 (hadmarea) | sgis-cli
```

---

## 차트 템플릿 (ECharts)

### 대상 템플릿
`line-chart.html` · `bar-rank.html` · `pie-share.html`

### `{{DATA}}` 플레이스홀더

JSON 오브젝트로 치환합니다.

```json
{
  "labels": ["2020", "2021", "2022", "2023", "2024"],
  "unit": "명",
  "series": [
    { "name": "서울특별시", "values": [9586195, 9472127, 9417469, 9384512, 9335444] },
    { "name": "부산광역시", "values": [3349016, 3324335, 3295760, 3279604, 3257256] }
  ],
  "chartType": "line"
}
```

#### DATA 필드 설명

| 필드 | 설명 | 필수 | 사용 템플릿 |
|------|------|:---:|------------|
| `labels` | X축 라벨 또는 항목명 | O | 전체 |
| `unit` | 단위 (명, 백만원, % 등) | O | 전체 |
| `series` | `[{name, values}]` 배열 | O | 전체 |
| `chartType` | `"line"` / `"bar"` | X | 확장용 |

**템플릿별 `series` 사용 방식:**
- `line-chart.html` — `series` 배열 전체 사용 (다중 시리즈 지원)
- `bar-rank.html` — `series[0]` 의 값으로 순위 막대 생성
- `pie-share.html` — `labels` → 항목명, `series[0].values` → 각 항목 값

### 차트 템플릿 사용 예시

```bash
# 1. sgis-cli로 데이터 조회
sgis data population --adm-cd 11 --year 2020 --format json > data.json

# 2. DATA JSON 구성 후 플레이스홀더 치환
#    (AI가 data.json을 읽어 위 DATA 형식으로 변환)
# 3. 브라우저로 결과 HTML 열기
```

---

## 지도 템플릿 (Leaflet + proj4js)

### 대상 템플릿
`map.html` · `map-choropleth.html`

### EPSG:5179 좌표계 처리 (중요)

SGIS 경계 API(`sgis boundary ...`)가 반환하는 GeoJSON 좌표는
**EPSG:5179 (UTM-K)** 투영 좌표계를 사용합니다.

```
EPSG:5179 정의:
+proj=tmerc +lat_0=38 +lon_0=127.5 +k=0.9996
+x_0=1000000 +y_0=2000000 +ellps=GRS80 +units=m +no_defs
```

Leaflet은 **WGS84 (EPSG:4326)** 경위도 좌표를 기대하므로
두 좌표계 간 변환이 필요합니다.

**이 템플릿의 처리 방식:**
1. CDN에서 **proj4js** + **proj4leaflet** 로드
2. EPSG:5179 정의를 `proj4.defs()`로 등록
3. 첫 번째 피처의 좌표 범위로 CRS를 자동 탐지
   - X좌표가 1 000–2 000 000 범위 → UTM-K로 판단
   - X좌표가 124–132 범위 → 이미 WGS84로 판단 (재투영 생략)
4. UTM-K로 판단된 경우 모든 좌표를 `proj4("EPSG:5179","WGS84", ...)` 로 재투영
5. 재투영된 WGS84 GeoJSON을 Leaflet `L.geoJSON()` 에 전달

> **주의:** `sgis boundary --format geojson` 출력(또는 Go의 `geojson.go`가
> WGS84로 정규화한 출력)을 그대로 `{{GEOJSON}}`에 주입하면 자동 처리됩니다.
> WGS84로 이미 변환된 GeoJSON을 주입해도 이중 변환 없이 안전하게 동작합니다.

### `{{GEOJSON}}` 플레이스홀더

GeoJSON `FeatureCollection` 오브젝트(또는 JSON 문자열)를 주입합니다.

```json
{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "properties": { "adm_nm": "서울특별시", "adm_cd": "11" },
      "geometry": {
        "type": "Polygon",
        "coordinates": [[[...], ...]]
      }
    }
  ]
}
```

### `map.html` 전용 플레이스홀더

| 플레이스홀더 | 설명 |
|-------------|------|
| `{{TITLE}}` | 지도 제목 |
| `{{SUBTITLE}}` | 부제목 |
| `{{GEOJSON}}` | GeoJSON FeatureCollection |
| `{{SOURCE}}` | 출처 |

피처 팝업/툴팁에는 다음 속성을 우선 표시합니다:
`adm_nm` → `name` → `SIG_KOR_NM` → `EMD_KOR_NM` → `bjdong_nm`

### `map-choropleth.html` 전용 플레이스홀더

`map.html` 플레이스홀더에 추가로 아래를 치환합니다.

| 플레이스홀더 | 설명 | 예시 |
|-------------|------|------|
| `{{VALUE_PROP}}` | 색상 매핑에 사용할 속성 키 | `pop_cnt`, `비율`, `value` |
| `{{UNIT}}` | 값의 단위 | `명`, `%`, `백만원` |

색상 스케일은 5단계 파란색 순차 램프(연→진)로 자동 계산됩니다.
데이터 없는 피처는 회색으로 표시됩니다.

### 지도 템플릿 사용 예시

```bash
# 1. 경계 GeoJSON 조회
sgis boundary hadmarea --adm-cd 11 --format geojson > seoul.geojson

# 2. {{GEOJSON}} 자리에 seoul.geojson 내용 주입
# 3. 단계구분도라면 각 feature.properties에 통계값 병합
# 4. 브라우저로 결과 HTML 열기
```

---

## AI가 데이터를 변환하는 방법

### 차트 (통계 → DATA JSON)

sgis-cli JSON 출력:
```json
[
  {"행정구역": "서울특별시", "시점": "2024", "수치값": "9335444", "단위": "명"},
  {"행정구역": "부산광역시", "시점": "2024", "수치값": "3257256", "단위": "명"}
]
```

→ DATA JSON:
```json
{
  "labels": ["서울특별시", "부산광역시"],
  "unit": "명",
  "series": [{ "name": "2024년 인구", "values": [9335444, 3257256] }]
}
```

### 지도 (경계 + 통계 → GeoJSON with properties)

```json
{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "properties": { "adm_nm": "서울특별시", "pop_cnt": 9335444 },
      "geometry": { "type": "Polygon", "coordinates": [[...]] }
    }
  ]
}
```

단계구분도에서는 `{{VALUE_PROP}}` = `"pop_cnt"`, `{{UNIT}}` = `"명"` 으로 치환합니다.

---

## CDN 의존성

| 라이브러리 | 버전 | CDN URL |
|-----------|------|---------|
| ECharts | 5.x | `https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js` |
| Leaflet CSS | 1.9.x | `https://cdn.jsdelivr.net/npm/leaflet@1.9/dist/leaflet.min.css` |
| Leaflet JS | 1.9.x | `https://cdn.jsdelivr.net/npm/leaflet@1.9/dist/leaflet.min.js` |
| proj4js | 2.11.x | `https://cdn.jsdelivr.net/npm/proj4@2.11/dist/proj4.js` |
| proj4leaflet | 1.0.2 | `https://cdn.jsdelivr.net/npm/proj4leaflet@1.0.2/src/proj4leaflet.js` |

모든 템플릿은 빌드 단계 없이 CDN 스크립트 태그로 완전히 자립합니다.
