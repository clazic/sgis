package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeToken은 테스트용 고정 토큰입니다.
const fakeToken = "test-access-token-1234"

// newTestClient는 httptest.Server URL과 가짜 토큰 공급자를 주입한 테스트용 Client를 반환합니다.
func newTestClient(serverURL string, token string) *Client {
	return &Client{
		baseURL:  serverURL + "/",
		getToken: func(_ context.Context) (string, error) { return token, nil },
		httpClient: &http.Client{
			Timeout: httpClientTimeout,
		},
	}
}

// TestGet_SuccessfulCall은 성공 응답(errCd=0)에서 result를 올바르게 반환하는지 검증합니다.
func TestGet_SuccessfulCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":{"tot_ppltn":9586195},"errCd":0,"errMsg":"Success","id":"API_0301","trId":"test"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, fakeToken)
	result, err := c.Get(context.Background(), "stats/population.json", map[string]string{"year": "2020", "adm_cd": "11"})
	if err != nil {
		t.Fatalf("예상치 못한 에러: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("result JSON 파싱 실패: %v", err)
	}
	if v, ok := data["tot_ppltn"]; !ok || v.(float64) != 9586195 {
		t.Errorf("tot_ppltn 값 불일치: %v", data)
	}
}

// TestGet_ErrCdNonZero는 errCd!=0 응답에서 error를 반환하는지 검증합니다.
func TestGet_ErrCdNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":null,"errCd":-100,"errMsg":"토큰이 만료되었습니다","id":"","trId":""}`))
	}))
	defer srv.Close()

	// 토큰 만료 에러는 재발급 후 재시도를 트리거하므로, 여기서는 재발급도 실패하도록 설정
	callCount := 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":null,"errCd":-999,"errMsg":"알 수 없는 오류","id":"","trId":""}`))
	}))
	defer srv2.Close()

	c := newTestClient(srv2.URL, fakeToken)
	_, err := c.Get(context.Background(), "stats/population.json", nil)
	if err == nil {
		t.Fatal("errCd!=0인데 에러가 반환되지 않았습니다")
	}
	if !strings.Contains(err.Error(), "errCd") {
		t.Errorf("에러 메시지에 errCd가 없습니다: %v", err)
	}
}

// TestGet_TokenInjected는 accessToken이 쿼리 파라미터로 주입되는지 검증합니다.
func TestGet_TokenInjected(t *testing.T) {
	var receivedToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.URL.Query().Get("accessToken")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":[],"errCd":0,"errMsg":"Success","id":"","trId":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, fakeToken)
	_, err := c.Get(context.Background(), "addr/stage.json", nil)
	if err != nil {
		t.Fatalf("예상치 못한 에러: %v", err)
	}
	if receivedToken != fakeToken {
		t.Errorf("accessToken 불일치: got=%q want=%q", receivedToken, fakeToken)
	}
}

// TestGet_429Retried는 429 응답이 최대 3회까지 재시도되는지 검증합니다.
func TestGet_429Retried(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// 3번째 시도에서 성공
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":{"ok":true},"errCd":0,"errMsg":"Success","id":"","trId":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, fakeToken)
	// backoff를 0으로 만들기 위해 initialBackoff를 직접 오버라이드하는 대신,
	// 테스트 클라이언트의 backoff를 최소화하기 위해 context로 타임아웃을 설정
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// backoff 시간이 길어 테스트가 느릴 수 있으므로, 429가 3회 이후 성공하는 경로 확인
	result, err := c.doGet(ctx, "stats/population.json", nil, fakeToken, false)
	if err != nil {
		t.Fatalf("재시도 후 성공해야 하는데 에러: %v", err)
	}
	if result == nil {
		t.Fatal("result가 nil입니다")
	}
	if attempts != 3 {
		t.Errorf("재시도 횟수 불일치: got=%d want=3", attempts)
	}
}

// TestGet_GeoJSONPassthrough는 .geojson 엔드포인트가 봉투 없이 body를 그대로 반환하는지 검증합니다.
func TestGet_GeoJSONPassthrough(t *testing.T) {
	geoBody := `{"type":"FeatureCollection","features":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(geoBody))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, fakeToken)
	result, err := c.Get(context.Background(), "boundary/hadmarea.geojson", map[string]string{"year": "2024", "adm_cd": "11"})
	if err != nil {
		t.Fatalf("예상치 못한 에러: %v", err)
	}
	if string(result) != geoBody {
		t.Errorf("GeoJSON passthrough 불일치:\ngot:  %s\nwant: %s", result, geoBody)
	}
}

// TestCall은 Call 편의 메서드가 Endpoint.Path를 올바르게 사용하는지 검증합니다.
func TestCall(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":[],"errCd":0,"errMsg":"Success","id":"","trId":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, fakeToken)
	ep, ok := FindEndpoint("code", "stage")
	if !ok {
		t.Fatal("stage 엔드포인트를 찾지 못했습니다")
	}

	_, err := c.Call(context.Background(), ep, nil)
	if err != nil {
		t.Fatalf("예상치 못한 에러: %v", err)
	}
	wantPath := "/" + ep.Path
	if receivedPath != wantPath {
		t.Errorf("경로 불일치: got=%q want=%q", receivedPath, wantPath)
	}
}
