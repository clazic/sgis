package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/relay"
)

const (
	relayPrefix   = "/sgis/https/sgisapi.mods.go.kr/OpenAPI3/"
	relayRunToken = "run-token-abc"
	// 사용자 자신의 키·토큰(중계 모드에서는 절대 나가면 안 됨)
	userKey    = "USERKEY-must-not-leak"
	userSecret = "USERSECRET-must-not-leak"
	userToken  = "user-own-access-token"
)

// relayHit은 가짜 중계가 받은 요청 하나입니다.
type relayHit struct {
	Path  string
	Query map[string]string
	Token string
	Raw   string
}

// fakeRelay는 pes-bridge 중계 흉내를 내는 httptest 서버입니다.
type fakeRelay struct {
	*httptest.Server
	mu   sync.Mutex
	hits []relayHit
	// handle이 nil이 아니면 기본 동작 대신 씁니다. true를 돌려주면 응답을 끝낸 것입니다.
	handle func(w http.ResponseWriter, r *http.Request) bool
}

func (f *fakeRelay) all() []relayHit {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]relayHit(nil), f.hits...)
}

func writeAuthOK(w http.ResponseWriter, token string) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"result": map[string]interface{}{
			"accessToken":   token,
			"accessTimeout": fmt.Sprintf("%d", time.Now().Add(4*time.Hour).UnixMilli()),
		},
		"errCd": 0, "errMsg": "Success",
	})
}

// newFakeRelay는 가짜 중계를 띄웁니다. 인증 요청에는 자리표시 번호에 맞춘 토큰
// ("relay-token-<i>")을, 그 밖에는 성공 봉투를 돌려줍니다.
func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	f := &fakeRelay{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := relayHit{Path: r.URL.Path, Query: map[string]string{}, Token: r.Header.Get("X-PES-Relay-Token"), Raw: r.URL.String()}
		for k, v := range r.URL.Query() {
			h.Query[k] = v[0]
		}
		f.mu.Lock()
		f.hits = append(f.hits, h)
		f.mu.Unlock()

		if f.handle != nil && f.handle(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/auth/authentication.json") {
			var i int
			_, _ = fmt.Sscanf(r.URL.Query().Get("consumer_key"), "__PES_KEY_%d__", &i)
			writeAuthOK(w, fmt.Sprintf("relay-token-%d", i))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"ok":true},"errCd":0,"errMsg":"Success"}`))
	}))
	t.Cleanup(f.Close)
	return f
}

// setupRelayEnv는 중계 모드 환경을 만듭니다. 설정 디렉토리는 임시 폴더로 돌리고,
// 거기에 사용자 자신의 키(config.yaml · 환경변수)와 토큰(token.json)을 미리 넣어 둡니다.
func setupRelayEnv(t *testing.T, relayURL, keys string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	config.SetConfigDirForTesting(dir)
	t.Cleanup(func() { config.SetConfigDirForTesting("") })

	cfg := fmt.Sprintf("consumer_key: %s\nconsumer_secret: %s\n", userKey, userSecret)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SGIS_CONSUMER_KEY", userKey)
	t.Setenv("SGIS_CONSUMER_SECRET", userSecret)

	// 사용자 자신의 유효한 토큰 캐시 (지문 = sha256(userKey)[:16] 과 무관하게, 쓰이면 안 됨)
	tok := fmt.Sprintf(`{"accessToken":%q,"expiresAt":%q,"issuedAt":%q,"credFingerprint":"whatever"}`,
		userToken, time.Now().Add(4*time.Hour).Format(time.RFC3339), time.Now().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(tok), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PES_RELAY_URL", relayURL)
	t.Setenv("PES_RELAY_TOKEN", relayRunToken)
	if keys == "<unset>" {
		t.Setenv("PES_RELAY_SGIS_KEYS", "")
		os.Unsetenv("PES_RELAY_SGIS_KEYS")
	} else {
		t.Setenv("PES_RELAY_SGIS_KEYS", keys)
	}
	return dir
}

// pinRelayStart는 키 풀 시작 번호를 고정합니다.
func pinRelayStart(t *testing.T, start int) {
	t.Helper()
	orig := relayStart
	relayStart = func(int) int { return start }
	t.Cleanup(func() { relayStart = orig })
}

// 주소 바꾸기 · 토큰 머리글 · 인증 요청의 자리표시 · 설정 파일 키 무시 · 토큰 캐시 분리.
func TestRelay_RewritesAuthAndDataRequests(t *testing.T) {
	f := newFakeRelay(t)
	dir := setupRelayEnv(t, f.URL, "1")

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient 실패: %v", err)
	}
	res, err := c.Get(context.Background(), "stats/population.json", map[string]string{"year": "2020", "adm_cd": "11"})
	if err != nil {
		t.Fatalf("Get 실패: %v", err)
	}
	if string(res) != `{"ok":true}` {
		t.Errorf("결과 불일치: %s", res)
	}

	hits := f.all()
	if len(hits) != 2 {
		t.Fatalf("중계 요청 수 = %d, 기대 2 (인증 + 데이터): %+v", len(hits), hits)
	}
	authHit, dataHit := hits[0], hits[1]

	if authHit.Path != relayPrefix+"auth/authentication.json" {
		t.Errorf("인증 요청 경로 불일치: %s", authHit.Path)
	}
	if authHit.Query["consumer_key"] != "__PES_KEY_0__" || authHit.Query["consumer_secret"] != "__PES_SECRET_0__" {
		t.Errorf("인증 요청에 자리표시가 없음: %v", authHit.Query)
	}
	if dataHit.Path != relayPrefix+"stats/population.json" {
		t.Errorf("데이터 요청 경로 불일치: %s", dataHit.Path)
	}
	if dataHit.Query["accessToken"] != "relay-token-0" || dataHit.Query["year"] != "2020" || dataHit.Query["adm_cd"] != "11" {
		t.Errorf("데이터 요청 질의 불일치: %v", dataHit.Query)
	}
	for _, h := range hits {
		if h.Token != relayRunToken {
			t.Errorf("X-PES-Relay-Token 불일치: %q (%s)", h.Token, h.Path)
		}
		for _, leak := range []string{userKey, userSecret, userToken} {
			if strings.Contains(h.Raw, leak) {
				t.Errorf("사용자 자신의 키·토큰이 중계 요청에 섞임: %s", h.Path)
			}
		}
	}

	// 사용자 자신의 token.json 은 그대로, 중계 토큰은 따로 저장
	own, _ := os.ReadFile(filepath.Join(dir, "token.json"))
	if !strings.Contains(string(own), userToken) || strings.Contains(string(own), "relay-token") {
		t.Error("사용자 자신의 token.json 이 바뀜")
	}
	relayCache, err := os.ReadFile(filepath.Join(dir, "token.relay.0.json"))
	if err != nil || !strings.Contains(string(relayCache), "relay-token-0") {
		t.Errorf("중계 토큰 캐시(token.relay.0.json)가 없음: %v", err)
	}

	// 같은 실행에서는 캐시를 다시 씀 (인증 요청 추가 없음)
	if _, err := c.Get(context.Background(), "addr/stage.json", nil); err != nil {
		t.Fatalf("두 번째 Get 실패: %v", err)
	}
	if n := len(f.all()); n != 3 {
		t.Errorf("두 번째 호출 뒤 중계 요청 수 = %d, 기대 3", n)
	}

	// 다른 실행(실행 토큰이 바뀜)에서는 남은 캐시를 쓰지 않고 다시 발급
	t.Setenv("PES_RELAY_TOKEN", "run-token-next")
	if _, err := c.Get(context.Background(), "addr/stage.json", nil); err != nil {
		t.Fatalf("세 번째 Get 실패: %v", err)
	}
	hits = f.all()
	if len(hits) != 5 || !strings.HasSuffix(hits[3].Path, "auth/authentication.json") {
		t.Errorf("실행이 바뀌었는데 토큰을 다시 발급하지 않음 (요청 %d개)", len(hits))
	}
}

// 키 수가 0 이거나 변수가 없으면 중계도 SGIS도 부르지 않고 규약의 안내 글로 끝납니다.
func TestRelay_NoKeys_NoNetworkCall(t *testing.T) {
	const want = "PES 에 SGIS 키가 없어요. PES 「도구」에서 키를 넣으세요."
	for _, keys := range []string{"0", "<unset>", "abc"} {
		f := newFakeRelay(t)
		setupRelayEnv(t, f.URL, keys)

		c, err := NewClient()
		if c != nil || !errors.Is(err, relay.ErrNoKeys) {
			t.Fatalf("[%s] NewClient 가 ErrNoKeys 를 주지 않음: %v", keys, err)
		}
		if err.Error() != want {
			t.Errorf("[%s] 안내 글 불일치: %q", keys, err.Error())
		}
		if _, _, err := config.GetCredentials(); !errors.Is(err, relay.ErrNoKeys) {
			t.Errorf("[%s] GetCredentials 가 설정 파일 키를 돌려줌: %v", keys, err)
		}
		if config.HasCredentials() {
			t.Errorf("[%s] HasCredentials 가 설정 파일 키를 봄", keys)
		}
		if n := len(f.all()); n != 0 {
			t.Errorf("[%s] 키가 없는데 중계를 %d번 부름", keys, n)
		}
	}
}

// 중계가 스스로 낸 오류는 error 글 그대로 돌려주고, 다른 쌍으로 다시 해 보지 않습니다.
func TestRelay_RelayError_NoRetry(t *testing.T) {
	for _, onAuth := range []bool{true, false} {
		f := newFakeRelay(t)
		f.handle = func(w http.ResponseWriter, r *http.Request) bool {
			isAuth := strings.HasSuffix(r.URL.Path, "/auth/authentication.json")
			if isAuth != onAuth {
				return false
			}
			w.Header().Set("X-PES-Relay-Error", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"PES 에 SGIS 키가 없어요 (중계)"}`))
			return true
		}
		setupRelayEnv(t, f.URL, "3")
		pinRelayStart(t, 0)

		c, err := NewClient()
		if err != nil {
			t.Fatalf("NewClient 실패: %v", err)
		}
		_, err = c.Get(context.Background(), "stats/population.json", nil)
		re, ok := err.(*relay.Error)
		if !ok {
			t.Fatalf("[onAuth=%v] 중계 오류가 그대로 오지 않음: %T %v", onAuth, err, err)
		}
		if re.Error() != "PES 에 SGIS 키가 없어요 (중계)" || re.Status != http.StatusConflict {
			t.Errorf("[onAuth=%v] 중계 오류 내용 불일치: %+v", onAuth, re)
		}
		want := 1 // 인증에서 막힘
		if !onAuth {
			want = 2 // 인증 1 + 데이터 1
		}
		if n := len(f.all()); n != want {
			t.Errorf("[onAuth=%v] 중계 요청 수 = %d, 기대 %d (다시 시도하면 안 됨)", onAuth, n, want)
		}
	}
}

// 쌍이 여러 개면 인증이 거부된 쌍을 건너뛰고 다음 쌍을 씁니다.
func TestRelay_PoolFailsOverToNextPair(t *testing.T) {
	f := newFakeRelay(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Query().Get("consumer_key") == "__PES_KEY_1__" {
			if r.URL.Query().Get("consumer_secret") != "__PES_SECRET_1__" {
				t.Errorf("키와 비밀의 번호가 다름: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{},"errCd":-401,"errMsg":"인증 정보가 존재하지 않습니다"}`))
			return true
		}
		return false
	}
	setupRelayEnv(t, f.URL, "3")
	pinRelayStart(t, 1)

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient 실패: %v", err)
	}
	if _, err := c.Get(context.Background(), "stats/population.json", nil); err != nil {
		t.Fatalf("다음 쌍으로 넘어가지 못함: %v", err)
	}
	hits := f.all()
	if len(hits) != 3 {
		t.Fatalf("중계 요청 수 = %d, 기대 3 (1번 인증 거부 → 2번 인증 → 데이터)", len(hits))
	}
	if hits[1].Query["consumer_key"] != "__PES_KEY_2__" || hits[1].Query["consumer_secret"] != "__PES_SECRET_2__" {
		t.Errorf("다음 쌍 자리표시 불일치: %v", hits[1].Query)
	}
	if hits[2].Query["accessToken"] != "relay-token-2" {
		t.Errorf("다음 쌍 토큰으로 호출하지 않음: %v", hits[2].Query)
	}
}

// 모든 쌍이 거부되면 CLI 설정 안내(sgis config …) 없이 실패를 알립니다.
func TestRelay_PoolAllPairsRejected(t *testing.T) {
	f := newFakeRelay(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{},"errCd":-401,"errMsg":"인증 정보가 존재하지 않습니다"}`))
		return true
	}
	setupRelayEnv(t, f.URL, "2")
	pinRelayStart(t, 0)

	c, _ := NewClient()
	_, err := c.Get(context.Background(), "stats/population.json", nil)
	if err == nil {
		t.Fatal("모든 쌍이 거부됐는데 성공함")
	}
	if !strings.Contains(err.Error(), "모든 키(2개)가 실패했어요") || strings.Contains(err.Error(), "sgis config") {
		t.Errorf("오류 글 불일치: %v", err)
	}
	if n := len(f.all()); n != 2 {
		t.Errorf("중계 요청 수 = %d, 기대 2", n)
	}
}

// 중계는 3xx 를 그대로 돌려줍니다. Location 이 기관 주소면 다시 중계 주소로 바꿔 요청합니다.
func TestRelay_RedirectToSGISHostGoesBackThroughRelay(t *testing.T) {
	f := newFakeRelay(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/OpenAPI3/old.json") {
			w.Header().Set("Location", "https://sgisapi.kostat.go.kr/OpenAPI3/new.json?moved=1")
			w.WriteHeader(http.StatusFound)
			return true
		}
		return false
	}
	setupRelayEnv(t, f.URL, "1")

	c, _ := NewClient()
	if _, err := c.Get(context.Background(), "old.json", nil); err != nil {
		t.Fatalf("Get 실패: %v", err)
	}
	hits := f.all()
	last := hits[len(hits)-1]
	if last.Path != "/sgis/https/sgisapi.kostat.go.kr/OpenAPI3/new.json" || last.Query["moved"] != "1" {
		t.Errorf("넘김 뒤 요청이 중계 주소가 아님: %s", last.Raw)
	}
	if last.Token != relayRunToken {
		t.Errorf("넘김 뒤 요청에 토큰 머리글이 없음")
	}
}

// PES_RELAY_URL 이 없으면 지금까지와 똑같이 동작합니다.
func TestRelay_Disabled_Unchanged(t *testing.T) {
	t.Setenv("PES_RELAY_URL", "")
	t.Setenv("PES_RELAY_TOKEN", "ignored")
	t.Setenv("PES_RELAY_SGIS_KEYS", "0")
	dir := t.TempDir()
	config.SetConfigDirForTesting(dir)
	t.Cleanup(func() { config.SetConfigDirForTesting("") })
	t.Setenv("SGIS_CONSUMER_KEY", userKey)
	t.Setenv("SGIS_CONSUMER_SECRET", userSecret)

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient 실패: %v", err)
	}
	if c.relayPairs != 0 {
		t.Error("중계 모드가 아닌데 키 풀이 켜짐")
	}
	if _, ok := c.httpClient.Transport.(*http.Transport); !ok {
		t.Errorf("중계 모드가 아닌데 전송이 바뀜: %T", c.httpClient.Transport)
	}
	k, s, err := config.GetCredentials()
	if err != nil || k != userKey || s != userSecret {
		t.Errorf("중계 모드가 아닌데 설정 키를 쓰지 않음: %v", err)
	}

	// 요청이 중계 주소로 바뀌지 않고, 토큰 머리글도 붙지 않음
	var gotPath, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotHeader = r.URL.Path, r.Header.Get("X-PES-Relay-Token")
		_, _ = w.Write([]byte(`{"result":{"ok":true},"errCd":0}`))
	}))
	defer srv.Close()
	c.baseURL = srv.URL + "/"
	c.getToken = func(context.Context) (string, error) { return fakeToken, nil }
	if _, err := c.Get(context.Background(), "stats/population.json", nil); err != nil {
		t.Fatalf("Get 실패: %v", err)
	}
	if gotPath != "/stats/population.json" || gotHeader != "" {
		t.Errorf("중계 모드가 아닌데 요청이 바뀜: %s %q", gotPath, gotHeader)
	}
}
