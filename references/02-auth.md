# 02 — 인증 흐름

## 개요

SGIS는 2단계 인증을 사용합니다:

1. **consumerKey + consumerSecret** → accessToken 발급 (4시간 유효)
2. **accessToken** → 모든 데이터 API 호출 시 쿼리파라미터로 전달

CLI가 이 흐름을 완전 자동화합니다. 사용자는 consumerKey/consumerSecret만 한 번 등록하면 됩니다.

---

## 인증 흐름 다이어그램

```
사용자
  │
  ▼
sgis config set-credential <KEY> <SECRET>
  │  → ~/.sgis/config.yaml (0o600) 저장
  │
  ▼
sgis data population ... (명령 실행)
  │
  ▼
internal/auth/auth.go — GetToken()
  ├─ ~/.sgis/token.json 있고 credential 지문 일치 + 잔여 > 24분?
  │   ├─ YES → 캐시 토큰 반환 (재발급 없음)
  │   └─ NO  → advisory 파일 락 획득 (token.lock)
  │              └─ double-check (락 대기 중 타 프로세스가 발급했을 수 있음)
  │                  ├─ 이미 유효 → 락 해제 후 반환
  │                  └─ 여전히 무효 → SGIS 인증 API 호출
  │                      └─ accessToken + accessTimeout 수신
  │                          └─ token.json atomic 저장 (0o600) → 락 해제 → 반환
  │
  ▼
?accessToken=<TOKEN> 쿼리파라미터로 API 호출
```

---

## 인증 엔드포인트 (실측 확인)

```
GET https://sgisapi.mods.go.kr/OpenAPI3/auth/authentication.json
    ?consumer_key=<KEY>
    &consumer_secret=<SECRET>
```

응답:
```json
{
  "result": {
    "accessToken": "<TOKEN>",
    "accessTimeout": "1782022524954"
  },
  "errCd": 0,
  "errMsg": "Success"
}
```

**중요**: `accessTimeout`은 **밀리초 단위 Unix epoch 절대시각** (문서의 "초" 설명은 오류).
`"1782022524954"` 는 JSON 문자열 — `int64`로 직접 언마샬 시 파싱 오류 발생.

---

## 토큰 캐시 파일

위치: `~/.sgis/token.json` (권한 0o600)

```json
{
  "accessToken": "<MASKED>",
  "expiresAt": 1782022524954,
  "accessTimeout": 14400000,
  "credFingerprint": "a3f8..."
}
```

- `credFingerprint`: consumerKey의 SHA-256 앞 8바이트 — credential 교체 시 캐시 자동 무효화
- `expiresAt`: ms epoch 절대 만료 시각
- 잔여 < `max(60s, 0.1 * lifetime)` = **24분** 전에 선제 재발급

---

## 동시 실행 안전성

여러 `sgis` 프로세스가 동시에 실행될 때 중복 발급을 방지합니다:

- **advisory 파일 락** (`~/.sgis/token.lock`)
  - Unix: `syscall.Flock` (golang.org/x/sys/unix)
  - Windows: `LockFileEx` (golang.org/x/sys/windows)
- 락 획득 프로세스만 발급, 나머지는 대기 후 캐시 읽기
- NFS 등 락 미지원 파일시스템: 경고 후 락 없이 진행 (하드 실패 없음)
- 크래시 시 OS가 fd 닫으며 자동 해제 (stale 락 없음)

---

## 환경변수 우선순위

```
SGIS_CONSUMER_KEY / SGIS_CONSUMER_SECRET (환경변수)
  > ~/.sgis/config.yaml (파일)
  > 오류 안내 (미설정 시)
```

CI/CD 환경에서는 환경변수 사용 권장 (파일 불필요):
```bash
export SGIS_CONSUMER_KEY="<KEY>"
export SGIS_CONSUMER_SECRET="<SECRET>"
sgis data population --adm-cd 11 --year 2020
```

---

## 에러 구분

| 상황 | 에러 타입 | 대응 |
|------|-----------|------|
| 잘못된 키/시크릿 (`errCd != 0`, HTTP 200) | `ErrInvalidCredential` | 재시도 무의미 → 발급 URL 안내 |
| 네트워크 실패 (연결 오류) | `ErrNetwork` | 재시도 가능 |
| HTTP 429 | rate limit | 자동 지수 백오프 (1·2·4s, 3회) |
