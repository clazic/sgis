package relay

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestEnabledAndKeyCount(t *testing.T) {
	t.Setenv(envURL, "")
	t.Setenv(envKeys, "3")
	if Enabled() {
		t.Fatal("PES_RELAY_URL 이 없는데 중계 모드로 판단함")
	}

	t.Setenv(envURL, "http://127.0.0.1:53999/")
	if !Enabled() {
		t.Fatal("PES_RELAY_URL 이 있는데 중계 모드가 아님")
	}
	for in, want := range map[string]int{"3": 3, " 2 ": 2, "0": 0, "": 0, "-1": 0, "abc": 0, "1.5": 0} {
		t.Setenv(envKeys, in)
		if got := KeyCount(); got != want {
			t.Errorf("KeyCount(%q) = %d, 기대 %d", in, got, want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	k, s := Placeholder(0)
	if k != "__PES_KEY_0__" || s != "__PES_SECRET_0__" {
		t.Errorf("0번 자리표시 불일치: %s / %s", k, s)
	}
	k, s = Placeholder(7)
	if k != "__PES_KEY_7__" || s != "__PES_SECRET_7__" {
		t.Errorf("7번 자리표시 불일치: %s / %s", k, s)
	}
}

func TestNoKeyMessageExact(t *testing.T) {
	const want = "PES 에 SGIS 키가 없어요. PES 「도구」에서 키를 넣으세요."
	if NoKeyMessage != want || ErrNoKeys.Error() != want {
		t.Errorf("안내 글이 규약과 다름: %q", NoKeyMessage)
	}
}

func TestRewrite(t *testing.T) {
	cases := []struct{ base, in, want string }{
		{
			"http://127.0.0.1:53999",
			"https://sgisapi.mods.go.kr/OpenAPI3/stats/population.json?year=2020&adm_cd=11",
			"http://127.0.0.1:53999/sgis/https/sgisapi.mods.go.kr/OpenAPI3/stats/population.json?year=2020&adm_cd=11",
		},
		{ // 끝의 '/' 와 포트
			"http://127.0.0.1:53999/",
			"http://sgisapi.mods.go.kr:8080/a/b.geojson",
			"http://127.0.0.1:53999/sgis/http/sgisapi.mods.go.kr:8080/a/b.geojson",
		},
		{ // 인코딩된 경로·질의는 그대로
			"http://127.0.0.1:53999",
			"https://sgisapi.mods.go.kr/a%2Fb/c?address=%EC%84%9C%EC%9A%B8+%EC%A2%85%EB%A1%9C",
			"http://127.0.0.1:53999/sgis/https/sgisapi.mods.go.kr/a%2Fb/c?address=%EC%84%9C%EC%9A%B8+%EC%A2%85%EB%A1%9C",
		},
	}
	for _, c := range cases {
		t.Setenv(envURL, c.base)
		u, _ := url.Parse(c.in)
		got, err := Rewrite(u)
		if err != nil {
			t.Fatalf("Rewrite(%s) 오류: %v", c.in, err)
		}
		if got.String() != c.want {
			t.Errorf("Rewrite(%s)\n  = %s\n기대 %s", c.in, got.String(), c.want)
		}
	}
}

func TestIsSGISHost(t *testing.T) {
	for host, want := range map[string]bool{
		"sgisapi.mods.go.kr":     true,
		"SGIS.mods.go.kr":        true,
		"sgisapi.kostat.go.kr":   true,
		"api.github.com":         false,
		"github.com":             false,
		"evilmods.go.kr":         false,
		"mods.go.kr.example.com": false,
		"127.0.0.1":              false,
	} {
		if got := IsSGISHost(host); got != want {
			t.Errorf("IsSGISHost(%q) = %v, 기대 %v", host, got, want)
		}
	}
}

func TestWrap_NotRelayMode_ReturnsBaseUnchanged(t *testing.T) {
	t.Setenv(envURL, "")
	base := &http.Transport{}
	if got := Wrap(base); got != http.RoundTripper(base) {
		t.Error("중계 모드가 아닌데 전송이 바뀜")
	}
	if got := Wrap(nil); got != nil {
		t.Error("중계 모드가 아닌데 nil 전송이 바뀜")
	}
}

func TestTransport_RewritesAndAddsToken(t *testing.T) {
	var gotPath, gotQuery, gotToken, gotMethod, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotMethod = r.URL.Path, r.URL.RawQuery, r.Method
		gotToken = r.Header.Get(headerToken)
		gotUA = r.Header.Get("X-Keep")
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	t.Setenv(envURL, srv.URL)
	t.Setenv(envToken, "run-token-1")

	req, _ := http.NewRequest(http.MethodGet, "https://sgisapi.mods.go.kr/OpenAPI3/x.json?a=1&b=2", nil)
	req.Header.Set("X-Keep", "kept")
	resp, err := (&http.Client{Transport: Wrap(nil)}).Do(req)
	if err != nil {
		t.Fatalf("요청 실패: %v", err)
	}
	resp.Body.Close()

	if gotPath != "/sgis/https/sgisapi.mods.go.kr/OpenAPI3/x.json" {
		t.Errorf("중계 경로 불일치: %s", gotPath)
	}
	if gotQuery != "a=1&b=2" {
		t.Errorf("질의가 바뀜: %s", gotQuery)
	}
	if gotToken != "run-token-1" {
		t.Errorf("토큰 머리글 불일치: %q", gotToken)
	}
	if gotMethod != http.MethodGet || gotUA != "kept" {
		t.Errorf("방식·머리글이 바뀜: %s %q", gotMethod, gotUA)
	}
	// 받은 요청 자체는 고치지 않아야 함
	if req.URL.Host != "sgisapi.mods.go.kr" || req.Header.Get(headerToken) != "" {
		t.Error("원래 요청이 바뀜")
	}
}

func TestTransport_NonSGISHostBypassesRelayWithoutToken(t *testing.T) {
	relayHits := 0
	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { relayHits++ }))
	defer relaySrv.Close()
	var gotToken string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get(headerToken)
	}))
	defer other.Close()
	t.Setenv(envURL, relaySrv.URL)
	t.Setenv(envToken, "run-token-1")

	resp, err := (&http.Client{Transport: Wrap(nil)}).Get(other.URL + "/releases/latest")
	if err != nil {
		t.Fatalf("요청 실패: %v", err)
	}
	resp.Body.Close()
	if relayHits != 0 {
		t.Error("기관이 아닌 호스트가 중계로 감")
	}
	if gotToken != "" {
		t.Error("기관이 아닌 호스트에 실행 토큰이 나감")
	}
}

func TestTransport_RelayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerError, "1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"허용되지 않은 호스트예요"}`)
	}))
	defer srv.Close()
	t.Setenv(envURL, srv.URL)

	_, err := (&http.Client{Transport: Wrap(nil)}).Get("https://sgisapi.mods.go.kr/x")
	re, ok := AsError(err)
	if !ok {
		t.Fatalf("중계 오류로 읽히지 않음: %v", err)
	}
	if re.Status != http.StatusForbidden || re.Message != "허용되지 않은 호스트예요" {
		t.Errorf("중계 오류 내용 불일치: %+v", re)
	}
	if !IsFatal(err) || !IsFatal(ErrNoKeys) || IsFatal(errors.New("다른 오류")) {
		t.Error("IsFatal 판단 오류")
	}
}

// 기관이 준 401/403(머리글 없음)은 중계 오류가 아니라 보통 응답으로 넘겨야 합니다.
func TestTransport_UpstreamErrorStatusIsNotRelayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv(envURL, srv.URL)

	resp, err := (&http.Client{Transport: Wrap(nil)}).Get("https://sgisapi.mods.go.kr/x")
	if err != nil {
		t.Fatalf("기관 응답이 오류로 바뀜: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("상태 코드 불일치: %d", resp.StatusCode)
	}
}
