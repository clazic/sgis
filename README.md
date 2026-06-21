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
