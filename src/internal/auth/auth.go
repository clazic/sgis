// Package auth는 SGIS API의 accessToken 발급, 캐싱, 만료 재발급을 담당합니다.
// token.json(0o600)에 파일 캐시하고 advisory 파일 락(token.lock)으로 동시 발급을 직렬화합니다.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/clazic/sgis/internal/config"
)

// 센티넬 에러: 호출자가 에러 종류에 따라 분기할 수 있도록 노출합니다.
var (
	// ErrInvalidCredential은 Consumer Key/Secret이 잘못되어 SGIS가 거부한 경우입니다.
	// 재시도해도 의미가 없으므로 사용자에게 재발급을 안내해야 합니다.
	ErrInvalidCredential = errors.New("SGIS 인증 거부: Consumer Key/Secret이 올바르지 않습니다")

	// ErrNetwork는 네트워크/전송 레벨 오류입니다. 재시도 가능합니다.
	ErrNetwork = errors.New("SGIS 인증 네트워크 오류")
)

// authBaseURL은 SGIS 인증 엔드포인트 기본 URL입니다.
// 테스트에서 httptest.Server URL로 오버라이드할 수 있습니다.
var authBaseURL = "https://sgisapi.mods.go.kr/OpenAPI3/auth/authentication.json"

// configDirFunc는 설정 디렉토리 반환 함수입니다. 테스트에서 오버라이드 가능합니다.
var configDirFunc = func() string {
	return config.ConfigDir()
}

// tokenCache는 token.json에 저장되는 캐시 구조체입니다.
type tokenCache struct {
	AccessToken     string    `json:"accessToken"`
	ExpiresAt       time.Time `json:"expiresAt"`
	IssuedAt        time.Time `json:"issuedAt"`
	CredFingerprint string    `json:"credFingerprint"` // sha256(consumerKey) hex[:16]
}

// authResponse는 SGIS 인증 API 응답 구조체입니다.
type authResponse struct {
	Result struct {
		AccessToken   string      `json:"accessToken"`
		AccessTimeout json.Number `json:"accessTimeout"` // 밀리초 Unix epoch 절대 만료 시각 (문자열 또는 숫자)
	} `json:"result"`
	ErrCd  int    `json:"errCd"`
	ErrMsg string `json:"errMsg"`
}

// tokenPath는 token.json 파일 경로를 반환합니다.
func tokenPath() string {
	return filepath.Join(configDirFunc(), "token.json")
}

// lockPath는 advisory lock 파일 경로를 반환합니다.
func lockPath() string {
	return filepath.Join(configDirFunc(), "token.lock")
}

// credFingerprint는 consumerKey의 SHA-256 해시 앞 16자리를 반환합니다.
func credFingerprint(consumerKey string) string {
	h := sha256.Sum256([]byte(consumerKey))
	return fmt.Sprintf("%x", h)[:16]
}

// needsRefresh는 현재 시각 기준으로 토큰 재발급이 필요한지 판단합니다.
// 재발급 임계: 잔여 시간 < max(60s, 0.1*(expiresAt-issuedAt))
func needsRefresh(tc *tokenCache) bool {
	now := time.Now()
	if now.After(tc.ExpiresAt) {
		return true
	}
	lifetime := tc.ExpiresAt.Sub(tc.IssuedAt)
	threshold := time.Duration(float64(lifetime) * 0.1)
	if threshold < 60*time.Second {
		threshold = 60 * time.Second
	}
	return tc.ExpiresAt.Sub(now) < threshold
}

// loadTokenCache는 token.json을 읽어 tokenCache를 반환합니다.
// 파일이 없거나 파싱 실패 시 nil을 반환합니다 (에러 없음 — 재발급으로 진행).
func loadTokenCache() *tokenCache {
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		return nil
	}
	var tc tokenCache
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil
	}
	return &tc
}

// saveTokenCache는 tokenCache를 token.json에 원자적으로 저장합니다 (0o600).
// 쓰기 순서: .tmp 파일 생성 → rename (원자 교체).
func saveTokenCache(tc *tokenCache) error {
	if err := config.EnsureConfigDir(); err != nil {
		return err
	}
	data, err := json.Marshal(tc)
	if err != nil {
		return fmt.Errorf("token 직렬화 실패: %w", err)
	}
	tp := tokenPath()
	tmp := tp + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("token 임시 파일 쓰기 실패: %w", err)
	}
	if err := os.Rename(tmp, tp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("token 파일 이동 실패: %w", err)
	}
	return nil
}

// issueToken은 SGIS 인증 엔드포인트에 GET 요청해 새 accessToken을 발급받습니다.
// errCd != 0 → ErrInvalidCredential, 네트워크 실패 → ErrNetwork.
func issueToken(ctx context.Context, client *http.Client, consumerKey, consumerSecret string) (*tokenCache, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authBaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: 요청 생성 실패: %v", ErrNetwork, err)
	}
	q := req.URL.Query()
	q.Set("consumer_key", consumerKey)
	q.Set("consumer_secret", consumerSecret)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	var ar authResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("%w: 응답 파싱 실패: %v", ErrNetwork, err)
	}

	// errCd != 0: SGIS가 명시적으로 거부한 경우 (잘못된 key/secret 등)
	if ar.ErrCd != 0 {
		return nil, fmt.Errorf("%w (errCd=%d, errMsg=%s)\n%s",
			ErrInvalidCredential, ar.ErrCd, ar.ErrMsg, config.NoCredentialMessage())
	}
	if ar.Result.AccessToken == "" {
		return nil, fmt.Errorf("%w: 응답에 accessToken 없음", ErrNetwork)
	}

	// accessTimeout은 밀리초 Unix epoch 절대 만료 시각입니다 (13자리).
	// 실제 SGIS API는 문자열로 반환하고 테스트 mock은 숫자로 반환하므로
	// json.Number로 받아 ParseInt로 변환합니다.
	timeoutMillis, parseErr := strconv.ParseInt(ar.Result.AccessTimeout.String(), 10, 64)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: accessTimeout 파싱 실패 (%q): %v", ErrNetwork, ar.Result.AccessTimeout.String(), parseErr)
	}
	issuedAt := time.Now()
	expiresAt := time.UnixMilli(timeoutMillis)

	tc := &tokenCache{
		AccessToken:     ar.Result.AccessToken,
		ExpiresAt:       expiresAt,
		IssuedAt:        issuedAt,
		CredFingerprint: credFingerprint(consumerKey),
	}
	return tc, nil
}

// GetToken은 유효한 accessToken을 반환합니다.
// 캐시가 유효하면 재사용하고, 만료/credential 변경 시 재발급합니다.
// 동시 호출은 advisory 파일 락(token.lock)으로 직렬화됩니다.
func GetToken(ctx context.Context) (string, error) {
	consumerKey, consumerSecret, err := config.GetCredentials()
	if err != nil {
		return "", err
	}
	fp := credFingerprint(consumerKey)
	client := &http.Client{Timeout: 15 * time.Second}

	// 락 전 빠른 캐시 확인 (락 없이 read-only)
	if tc := loadTokenCache(); tc != nil {
		if tc.CredFingerprint == fp && !needsRefresh(tc) {
			return tc.AccessToken, nil
		}
	}

	// advisory 파일 락 획득 (토큰 발급 임계 영역 직렬화)
	lp := lockPath()
	lockFile, lockErr := os.OpenFile(lp, os.O_CREATE|os.O_RDWR, 0o600)
	if lockErr != nil {
		log.Printf("advisory lock 파일 열기 실패: %v; lock-free로 진행합니다", lockErr)
	}

	if lockFile != nil {
		if err := acquireLock(lockFile); err != nil {
			log.Printf("advisory lock 획득 실패: %v; lock-free로 진행합니다", err)
			_ = lockFile.Close()
			lockFile = nil
		}
	}

	// 락 해제는 함수 종료 시 수행
	defer func() {
		if lockFile != nil {
			_ = releaseLock(lockFile)
			_ = lockFile.Close()
		}
	}()

	// 락 획득 후 double-check: 다른 프로세스가 이미 발급했을 수 있음
	if tc := loadTokenCache(); tc != nil {
		if tc.CredFingerprint == fp && !needsRefresh(tc) {
			return tc.AccessToken, nil
		}
	}

	// 실제 발급
	tc, err := issueToken(ctx, client, consumerKey, consumerSecret)
	if err != nil {
		return "", err
	}

	// atomic write (락 보유 상태에서)
	if err := saveTokenCache(tc); err != nil {
		// 저장 실패해도 이번 호출은 성공으로 처리 (토큰은 유효함)
		log.Printf("token 캐시 저장 실패: %v", err)
	}

	return tc.AccessToken, nil
}
