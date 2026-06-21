# SGIS CLI 프로젝트 규칙

## 프로젝트 개요

SGIS(통계지리정보서비스) Open API를 사용하는 멀티플랫폼 Go CLI. kosis 스킬 구조를 모델로 하되 다음이 다르다:
- **인증**: consumerKey + consumerSecret → accessToken 자동 발급·캐싱 (4시간, `~/.sgis/token.json`)
- **데이터**: 통계 + 지오공간 경계(GeoJSON) + 지오코딩/좌표변환
- **명령 구조**: 카테고리 그룹 (`sgis data`, `sgis boundary`, `sgis geocode`, `sgis code`)

## 빌드 규칙 (필수)

```bash
cd src && make build
```

- 5개 플랫폼 전부 `CGO_ENABLED=0` 빌드 (sqlite/parquet 미사용 — 순수 Go)
- 산출물: `src/bin/` + 프로젝트 루트 `apps/` 두 곳에 자동 배치
- 파일명: `sgis-darwin-arm64` / `sgis-darwin-amd64` / `sgis-linux-amd64` / `sgis-linux-arm64` / `sgis-windows-amd64.exe`
- 현재 플랫폼만 빠르게: `make native`
- 테스트: `CGO_ENABLED=0 go test ./...`

## 계획 수립 규칙 (필수)

- 모든 계획서·설계 문서는 `.plan/` 폴더에 작성
- 파일명 형식: `.plan/YYYY-MM-DD-HH:MM:SS-제목.md`
- `.plan/`은 개발 전용 — gitignore됨, 배포 산출물에 포함하지 않음

## Playwright 사용 규칙

- 산출물은 **프로젝트 루트의 `.playwright/` 폴더 안에만** 저장
- 모든 playwright 명령 실행 전 `cd <프로젝트루트>/.playwright` 후 실행
- `.playwright/`, `.playwright-cli/`는 gitignore 등록됨

## 크로스플랫폼 규칙 (필수)

- 경로: `filepath.Join()` / `filepath.FromSlash()` 사용 — 슬래시 하드코딩 금지
- 줄 끝: `split(/\r?\n/)` (Windows `\r\n` 대응)
- 임시 폴더: `.` 접두사 숨김 폴더, `os.MkdirTemp()` 또는 `os.TempDir()` 사용
- CLI 탐색: Unix `which` / Windows `where` 분기 (`runtime.GOOS`)
- 프로세스 종료: Windows `child.Kill()` / Unix `child.Signal(syscall.SIGTERM)` 분기

## Golden Rules

변경 불가 원칙:
- API 자격증명(consumerKey/consumerSecret/accessToken)을 코드·로그·에러 메시지에 절대 하드코딩하지 않는다
- `--help` 출력은 구현 계약의 일부 — 플래그 변경 시 help 문자열과 함께 수정
- `CGO_ENABLED=0` 불변식 — sqlite/parquet 의존 도입 금지 (CGO 가드 스텝이 CI에서 강제)
- `src/` 코드 수정 후에는 반드시 `cd src && make build` + `go test ./...` 실행

해야 할 것:
- 변경 전 관련 파일과 인접 모듈을 읽고 맥락 확인
- `.plan/` 계획서와 실제 코드를 일치시킨다
- 에러 발생 시 원인 파악 우선 — 동일 에러 3회 이상 실패 시 접근 방식 재검토

하면 안 되는 것:
- `~/.sgis/config.yaml`, `~/.sgis/token.json` 내용을 로그/출력에 노출
- 계획서 없이 다중 파일을 한 번에 변경
- sqlite, parquet, bubbletea TUI 의존 추가 (deferred)

## 인증 아키텍처 요약

```
internal/auth/auth.go       — GetToken(): 캐시→발급→저장 (advisory 파일 락)
internal/auth/lock_unix.go  — syscall.Flock (build tag: !windows)
internal/auth/lock_windows.go — LockFileEx (build tag: windows)
~/.sgis/token.json          — 캐시 (0o600): {accessToken, expiresAt, credFingerprint}
~/.sgis/token.lock          — advisory 락 파일 (형제 파일)
```

토큰 재발급 임계: `max(60s, 0.1 * 14400s) = 24분` 전 선제 갱신.

## 참조 문서

- 구현 계획: `.omc/plans/sgis-cli-skill-plan.md`
- 스킬 가이드: `SKILL.md`
- 엔드포인트 카탈로그: `references/sgis-endpoints.md`
- 행정구역 코드표: `references/region-codes.md`
- 학습 로그: `LEARNINGS.md`
