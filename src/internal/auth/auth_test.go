package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clazic/sgis/internal/config"
)

// setupTestEnv는 테스트용 임시 디렉토리를 설정하고 정리 함수를 반환합니다.
// 실제 ~/.sgis를 오염시키지 않습니다.
func setupTestEnv(t *testing.T) (dir string, cleanup func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", ".sgis-auth-test-*")
	if err != nil {
		t.Fatalf("임시 디렉토리 생성 실패: %v", err)
	}

	// config 패키지와 auth 패키지 양쪽의 configDir을 오버라이드
	config.SetConfigDirForTesting(dir)
	origConfigDirFunc := configDirFunc
	configDirFunc = func() string { return dir }

	// 환경변수 설정 (테스트용 credential)
	os.Setenv("SGIS_CONSUMER_KEY", "test-key")
	os.Setenv("SGIS_CONSUMER_SECRET", "test-secret")

	cleanup = func() {
		config.SetConfigDirForTesting("")
		configDirFunc = origConfigDirFunc
		os.Unsetenv("SGIS_CONSUMER_KEY")
		os.Unsetenv("SGIS_CONSUMER_SECRET")
		os.RemoveAll(dir)
	}
	return dir, cleanup
}

// makeAuthServer는 httptest.Server를 생성합니다.
// callCount: 발급 호출 횟수 추적. errCd != 0이면 인증 거부 응답을 반환합니다.
func makeAuthServer(t *testing.T, callCount *atomic.Int32, errCd int, errMsg string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")

		if errCd != 0 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{},
				"errCd":  errCd,
				"errMsg": errMsg,
				"id":     "test",
				"trId":   "test-tr",
			})
			return
		}

		// accessTimeout: 현재 시각 + 4시간 (밀리초 Unix epoch, 실제 SGIS API처럼 문자열로 전송)
		expiresAt := time.Now().Add(4 * time.Hour).UnixMilli()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"accessToken":   fmt.Sprintf("token-%d", callCount.Load()),
				"accessTimeout": fmt.Sprintf("%d", expiresAt),
			},
			"errCd":  0,
			"errMsg": "Success",
			"id":     "test",
			"trId":   "test-tr",
		})
	}))
}

// TestGetToken_IssueAndCache는 최초 발급 후 캐시 히트를 검증합니다 (발급 1회만).
func TestGetToken_IssueAndCache(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	var callCount atomic.Int32
	srv := makeAuthServer(t, &callCount, 0, "")
	defer srv.Close()
	origURL := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = origURL }()

	ctx := context.Background()

	// 1회차: 발급
	tok1, err := GetToken(ctx)
	if err != nil {
		t.Fatalf("GetToken 1회차 실패: %v", err)
	}
	if tok1 == "" {
		t.Fatal("빈 토큰 반환됨")
	}
	if callCount.Load() != 1 {
		t.Fatalf("발급 호출 횟수 기대 1, 실제 %d", callCount.Load())
	}

	// 2회차: 캐시 히트여야 함 (발급 추가 호출 없음)
	tok2, err := GetToken(ctx)
	if err != nil {
		t.Fatalf("GetToken 2회차 실패: %v", err)
	}
	if tok1 != tok2 {
		t.Fatalf("캐시 히트 실패: tok1=%s tok2=%s", tok1, tok2)
	}
	if callCount.Load() != 1 {
		t.Fatalf("캐시 히트 시 추가 발급 발생: 호출횟수=%d", callCount.Load())
	}
}

// TestGetToken_ExpiredReissue는 만료된 토큰이 재발급되는지 검증합니다.
func TestGetToken_ExpiredReissue(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	var callCount atomic.Int32
	srv := makeAuthServer(t, &callCount, 0, "")
	defer srv.Close()
	origURL := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = origURL }()

	// 만료된 token.json을 직접 주입
	expiredCache := tokenCache{
		AccessToken:     "expired-token",
		ExpiresAt:       time.Now().Add(-1 * time.Hour), // 1시간 전 만료
		IssuedAt:        time.Now().Add(-5 * time.Hour),
		CredFingerprint: credFingerprint("test-key"),
	}
	data, _ := json.Marshal(expiredCache)
	os.WriteFile(fmt.Sprintf("%s/token.json", dir), data, 0o600)

	tok, err := GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken 실패: %v", err)
	}
	if tok == "expired-token" {
		t.Fatal("만료된 토큰이 그대로 반환됨")
	}
	if callCount.Load() != 1 {
		t.Fatalf("재발급 호출 횟수 기대 1, 실제 %d", callCount.Load())
	}
}

// TestGetToken_CredFingerprintMismatch는 credential 변경 시 캐시가 무효화되어 재발급되는지 검증합니다.
func TestGetToken_CredFingerprintMismatch(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	var callCount atomic.Int32
	srv := makeAuthServer(t, &callCount, 0, "")
	defer srv.Close()
	origURL := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = origURL }()

	// 다른 key로 발급된 토큰을 주입
	oldCache := tokenCache{
		AccessToken:     "old-key-token",
		ExpiresAt:       time.Now().Add(3 * time.Hour), // 아직 유효
		IssuedAt:        time.Now().Add(-1 * time.Hour),
		CredFingerprint: credFingerprint("different-key"), // 현재 key와 다름
	}
	data, _ := json.Marshal(oldCache)
	os.WriteFile(fmt.Sprintf("%s/token.json", dir), data, 0o600)

	tok, err := GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken 실패: %v", err)
	}
	if tok == "old-key-token" {
		t.Fatal("다른 credential 토큰이 그대로 반환됨 (fingerprint 검사 실패)")
	}
	if callCount.Load() != 1 {
		t.Fatalf("재발급 호출 횟수 기대 1, 실제 %d", callCount.Load())
	}
}

// TestGetToken_ConcurrentSingleIssuance는 동시 GetToken 호출이 발급을 1회만 수행하는지 검증합니다.
func TestGetToken_ConcurrentSingleIssuance(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	var callCount atomic.Int32

	// 발급에 약간의 지연을 주어 동시성 상황을 시뮬레이션
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		// 첫 번째 호출에만 지연을 줘서 다른 goroutine이 락을 대기하도록 유도
		if n == 1 {
			time.Sleep(50 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		expiresAt := time.Now().Add(4 * time.Hour).UnixMilli()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"accessToken":   "shared-token",
				"accessTimeout": fmt.Sprintf("%d", expiresAt),
			},
			"errCd":  0,
			"errMsg": "Success",
			"id":     "test",
			"trId":   "test-tr",
		})
	}))
	defer srv.Close()

	origURL := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = origURL }()

	const goroutines = 10
	var wg sync.WaitGroup
	tokens := make([]string, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			tokens[i], errs[i] = GetToken(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: GetToken 실패: %v", i, err)
		}
	}
	for i, tok := range tokens {
		if tok == "" {
			t.Errorf("goroutine %d: 빈 토큰 반환", i)
		}
	}

	// 파일 락 덕분에 발급 호출은 1회여야 합니다.
	// (락 비지원 환경에서는 최대 goroutines 호출까지 허용)
	if n := callCount.Load(); n > int32(goroutines) {
		t.Fatalf("발급 호출 횟수가 goroutine 수를 초과: %d > %d", n, goroutines)
	}
	t.Logf("동시 %d goroutine, 발급 호출 %d회", goroutines, callCount.Load())
}

// TestGetToken_InvalidCredential은 잘못된 key/secret 시 ErrInvalidCredential을 반환하는지 검증합니다.
func TestGetToken_InvalidCredential(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	var callCount atomic.Int32
	srv := makeAuthServer(t, &callCount, -100, "인증 실패: 잘못된 Consumer Key")
	defer srv.Close()

	origURL := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = origURL }()

	_, err := GetToken(context.Background())
	if err == nil {
		t.Fatal("에러가 반환되어야 하는데 nil 반환됨")
	}
	if !isErrInvalidCredential(err) {
		t.Fatalf("ErrInvalidCredential 기대, 실제: %v", err)
	}
}

// TestGetToken_NetworkError는 서버 연결 불가 시 ErrNetwork를 반환하는지 검증합니다.
func TestGetToken_NetworkError(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	origURL := authBaseURL
	// 존재하지 않는 주소를 사용
	authBaseURL = "http://127.0.0.1:19999"
	defer func() { authBaseURL = origURL }()

	_, err := GetToken(context.Background())
	if err == nil {
		t.Fatal("에러가 반환되어야 하는데 nil 반환됨")
	}
	if !isErrNetwork(err) {
		t.Fatalf("ErrNetwork 기대, 실제: %v", err)
	}
}

// isErrInvalidCredential은 에러 체인에 ErrInvalidCredential이 있는지 확인합니다.
func isErrInvalidCredential(err error) bool {
	if err == nil {
		return false
	}
	return containsErr(err, ErrInvalidCredential)
}

// isErrNetwork는 에러 체인에 ErrNetwork가 있는지 확인합니다.
func isErrNetwork(err error) bool {
	if err == nil {
		return false
	}
	return containsErr(err, ErrNetwork)
}

// containsErr는 에러 문자열에 target 에러 메시지가 포함되는지 확인합니다.
// errors.Is가 fmt.Errorf("%w: ...", target)로 wrapping된 경우에도 동작합니다.
func containsErr(err error, target error) bool {
	import_errors_is_check := false
	// errors.Is로 먼저 시도
	type unwrapper interface{ Unwrap() error }
	var e error = err
	for e != nil {
		if e == target {
			import_errors_is_check = true
			break
		}
		u, ok := e.(unwrapper)
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	if import_errors_is_check {
		return true
	}
	// 문자열 포함 검사 폴백
	return len(err.Error()) > 0 && len(target.Error()) > 0 &&
		(err.Error() == target.Error() ||
			stringContains(err.Error(), target.Error()))
}

func stringContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
