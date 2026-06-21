# Project Context & Operations

이 저장소는 SGIS CLI 스킬 구현 워크스페이스다. 핵심 산출물은 `src/` 아래의 Go 기반 CLI이며, 스킬 문서·엔드포인트 카탈로그·행정구역 코드표·설치 스크립트·차트 템플릿을 함께 관리한다.

기술 스택:
- Go 1.26.x / Cobra CLI / Viper 설정 관리
- 출력: table / JSON / CSV / GeoJSON / XLSX (순수 Go — CGO 없음)
- 인증: consumerKey + consumerSecret → accessToken 파일 캐시 (`~/.sgis/token.json`)
- Markdown 기반 운영 문서

Operational Commands:
- 빌드(필수): `cd src && make build` — 5개 플랫폼 전부 CGO_ENABLED=0
- 테스트: `CGO_ENABLED=0 go test ./...`
- 현재 플랫폼만: `cd src && make native`
- help 검증: `go run . --help` / `go run . data --help` / `go run . boundary --help`

---

# Golden Rules

변경 불가:
- `src/` 코드 수정 후 항상 `make build` + `go test ./...` 실행
- `--help`는 구현 계약 — 플래그 변경 시 help 문자열과 동시에 수정
- `CGO_ENABLED=0` 불변식 — sqlite/parquet/bubbletea 의존 금지
- credential(consumerKey/secret/accessToken)을 코드·로그·에러 메시지에 절대 노출하지 않음
- 계획서·설계 문서는 `.plan/YYYY-MM-DD-HH:MM:SS-제목.md` 형식으로 `.plan/` 에 작성

해야 할 것:
- 작업 전 `LEARNINGS.md` 읽기, 작업 후 새 발견 APPEND
- 변경 전 관련 파일·인접 모듈 맥락 확인
- 다중 파일 변경 시 `.plan/` 계획서 먼저 작성

하면 안 되는 것:
- `~/.sgis/` 내 파일(config.yaml, token.json) 내용을 출력·로그에 노출
- sqlite, parquet, TUI(bubbletea) 의존 추가 (deferred)
- `.plan/` 파일을 배포 산출물에 포함 (gitignore 대상)
- Playwright 산출물을 `.playwright/` 외부에 저장

---

# Standards & References

참조 문서:
- 구현 계획: `.omc/plans/sgis-cli-skill-plan.md`
- 스킬 가이드: `SKILL.md`
- 학습 로그: `LEARNINGS.md`
- 엔드포인트 카탈로그: `references/sgis-endpoints.md` (canonical reviewed-scrape record)
- 행정구역 코드표: `references/region-codes.md`

코딩 기준:
- Go 코드는 `gofmt` 기준
- 경로는 `filepath.Join()` 사용 — OS별 구분자 자동 처리
- 에러는 `ErrInvalidCredential` (재시도 무의미) vs `ErrNetwork` (재시도 가능) 명확히 분기
- 토큰·시크릿은 마스킹 후 로그 (`maskSecret()`)

Git 전략:
- 작은 단위로 의도적인 변경
- 커밋 메시지 형식: `type: 설명` — type은 feat/fix/chore/refactor/docs/perf

---

# Context Map (Action-Based Routing)

- **[CLI 명령어 계층](src/cmd/AGENTS.md)** — Cobra 명령, 플래그, help, 그룹 서브커맨드(data/boundary/geocode/code) 수정 시
- **[내부 구현 패키지](src/internal/AGENTS.md)** — auth/api/config/cache/output/chart 등 내부 로직 수정 시
- **[엔드포인트 레지스트리](references/sgis-endpoints.md)** — API 경로·파라미터 추가·변경 시 (canonical record)
- **[행정구역 코드표](references/region-codes.md)** — 시도/시군구 코드 갱신 시 (`sgis code stage` 라이브 재수집)

---

# Module Structure

```
sgis/
├── src/                    # Go 소스
│   ├── main.go
│   ├── cmd/                # Cobra 명령 (root, data, boundary, geocode, code, config, update)
│   ├── internal/
│   │   ├── auth/           # accessToken 발급·캐싱·파일락 [신규, kosis 대응 없음]
│   │   ├── api/            # HTTP 클라이언트 + 정적 엔드포인트 레지스트리
│   │   ├── config/         # ~/.sgis/config.yaml 관리
│   │   ├── cache/          # 메타/코드 응답 파일 캐시
│   │   ├── output/         # table/json/csv/geojson/xlsx 포맷터
│   │   └── chart/          # go-echarts 차트 + Leaflet 지도 HTML
│   ├── Makefile
│   └── bin/                # 빌드 산출물 (gitignore)
├── scripts/                # install.sh / install.ps1 / uninstall.*
├── templates/              # HTML 차트·지도 템플릿
├── references/             # 엔드포인트 카탈로그, 행정구역 코드표, 가이드 문서
├── apps/                   # 빌드 배포 바이너리 (gitignore)
├── SKILL.md                # AI 스킬 정의 (frontmatter + 명령어 가이드)
├── LEARNINGS.md            # 오답노트 (조회 전 READ, 작업 후 APPEND)
├── CLAUDE.md               # 프로젝트 작업 규칙
├── AGENTS.md               # 이 파일
└── VERSION                 # 현재 버전 (예: v0.1.0)
```

---

# Tech Stack & Constraints

- Go module: `github.com/clazic/sgis`
- CLI 프레임워크: Cobra
- 설정: Viper (`~/.sgis/config.yaml`)
- 출력 포맷: table / JSON / CSV / GeoJSON / XLSX (excelize — 순수 Go)
- 차트: go-echarts (HTML) + Leaflet (지도)
- **CGO_ENABLED=0 필수** — sqlite/parquet/mattn 의존 없음

Constraints:
- `go test ./...`를 통과하지 못하는 변경은 미완으로 본다
- auth 패키지는 httptest mock으로만 테스트 (CI가 실 SGIS 발급 rate에 닿지 않도록)
- boundary 좌표는 UTM-K(EPSG:5179) — Leaflet 지도 사용 시 WGS84 재투영 필요

---

# Implementation Patterns

- **명령 추가**: `src/cmd/<그룹>.go`에 부모 AddCommand → 자식 명령 struct 추가 → `internal/api/endpoints.go` registry 참조
- **엔드포인트 추가**: `references/sgis-endpoints.md` 먼저 갱신(canonical) → `internal/api/endpoints.go` 정적 registry 반영
- **출력 포맷 수정**: `internal/output/formatter.go` dispatch → 개별 포맷 파일 (`table.go`, `geojson.go` 등)
- **auth 흐름**: `internal/auth/auth.go` GetToken() → token.json 캐시 → 만료 시 재발급 → advisory 파일 락(lock_unix.go/lock_windows.go)

# Testing Strategy

- 전체 테스트: `CGO_ENABLED=0 go test ./...`
- auth 테스트: httptest mock (`auth_test.go`) — 발급/캐시히트/만료/동시발급/credential 지문 검증
- help 검증: `go run . --help` / `go run . data --help` / `go run . boundary --help`
- 실 API 호출 검증: `sgis code stage` (토큰 자동 발급 + 시도 17개 반환 확인)
