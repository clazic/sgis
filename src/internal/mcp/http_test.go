package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`

type httpClient struct {
	t       *testing.T
	url     string
	session string
	headers map[string]string
}

func newHTTPTest(t *testing.T, s *Server, o HTTPOptions) (*httptest.Server, *httpClient) {
	t.Helper()
	ts := httptest.NewServer(s.HTTPHandler(o))
	t.Cleanup(ts.Close)
	return ts, &httpClient{t: t, url: ts.URL + "/mcp", headers: map[string]string{}}
}

func (c *httpClient) do(method, body string, extra map[string]string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set(headerSession, c.session)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for k, v := range extra {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("HTTP: %v", err)
	}
	return resp
}

func (c *httpClient) initialize() {
	c.t.Helper()
	resp := c.do("POST", initBody, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		c.t.Fatalf("initialize: %d", resp.StatusCode)
	}
	c.session = resp.Header.Get(headerSession)
	if len(c.session) < 32 {
		c.t.Fatalf("Mcp-Session-Id 가 짧거나 없음: %q", c.session)
	}
}

func readJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]interface{}
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON 아님(%d): %s", resp.StatusCode, b)
	}
	return m
}

// readSSE는 SSE 이벤트의 data 들을 차례로 돌려줍니다.
func readSSE(t *testing.T, resp *http.Response) []map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var out []map[string]interface{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m); err != nil {
				t.Fatalf("SSE data JSON 아님: %s", line)
			}
			out = append(out, m)
		}
	}
	return out
}

func TestHTTPFlowWithoutKeys(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: discard()})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	c.initialize()

	r := c.do("POST", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, map[string]string{headerProtocol: "2025-06-18"})
	r.Body.Close()
	if r.StatusCode != http.StatusAccepted {
		t.Errorf("알림만 온 POST 는 202: %d", r.StatusCode)
	}

	m := readJSON(t, c.do("POST", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, map[string]string{headerProtocol: "2025-06-18"}))
	if n := len(m["result"].(map[string]interface{})["tools"].([]interface{})); n != 15 {
		t.Errorf("도구 15개 기대: %d", n)
	}

	m = readJSON(t, c.do("POST", callLine(3, "sgis_admin_codes", `{}`, ""), nil))
	text, isErr := resultText(t, m)
	if !isErr || !strings.Contains(text, "API 키가 없어요") {
		t.Errorf("키 없음 오류: %v %q", isErr, text)
	}

	// 배치
	resp := c.do("POST", `[{"jsonrpc":"2.0","id":4,"method":"ping"},{"jsonrpc":"2.0","id":5,"method":"tools/list"}]`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var arr []map[string]interface{}
	if err := json.Unmarshal(b, &arr); err != nil || len(arr) != 2 {
		t.Errorf("배치 응답 2개 기대: %s", b)
	}

	// 세션 끝
	r = c.do("DELETE", "", nil)
	r.Body.Close()
	if r.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE: %d", r.StatusCode)
	}
	r = c.do("POST", `{"jsonrpc":"2.0","id":6,"method":"ping"}`, nil)
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("끝난 세션은 404: %d", r.StatusCode)
	}
}

func TestHTTPSessionRequired(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: discard()})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	r := c.do("POST", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("세션 머리글 없으면 400: %d", r.StatusCode)
	}
	r = c.do("POST", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, map[string]string{headerSession: "deadbeef"})
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("모르는 세션은 404: %d", r.StatusCode)
	}
	c.initialize()
	r = c.do("POST", `{"jsonrpc":"2.0","id":2,"method":"ping"}`, map[string]string{headerProtocol: "1999-01-01"})
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("모르는 MCP-Protocol-Version 은 400: %d", r.StatusCode)
	}
	var many []string
	for i := 0; i < 11; i++ {
		many = append(many, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`, i+10))
	}
	r = c.do("POST", "["+strings.Join(many, ",")+"]", nil)
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("배치 11개는 400: %d", r.StatusCode)
	}
	r = c.do("POST", `{"jsonrpc":"2.0","id":3,"method":"ping","params":{"x":"`+strings.Repeat("a", maxBodyBytes)+`"}}`, nil)
	r.Body.Close()
	if r.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("1MB 넘으면 413: %d", r.StatusCode)
	}
}

func TestHTTPSessionExpires(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: discard()})
	_, c := newHTTPTest(t, s, HTTPOptions{SessionTTL: 50 * time.Millisecond})
	c.initialize()
	time.Sleep(120 * time.Millisecond)
	r := c.do("POST", `{"jsonrpc":"2.0","id":2,"method":"ping"}`, nil)
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("만료 세션은 404: %d", r.StatusCode)
	}
}

func TestHTTPTokenAuth(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: discard()})
	_, c := newHTTPTest(t, s, HTTPOptions{Token: "s3cret-token"})
	for _, auth := range []string{"", "Bearer wrong", "s3cret-token", "Bearer s3cret-token2"} {
		r := c.do("POST", initBody, map[string]string{"Authorization": auth})
		r.Body.Close()
		if r.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization %q 는 401: %d", auth, r.StatusCode)
		}
	}
	c.headers["Authorization"] = "Bearer s3cret-token"
	c.initialize()
}

func TestCheckBind(t *testing.T) {
	for _, a := range []string{"127.0.0.1:8080", "localhost:1", "[::1]:9"} {
		if err := CheckBind(a, ""); err != nil {
			t.Errorf("%s 는 토큰 없이 허용: %v", a, err)
		}
	}
	for _, a := range []string{"0.0.0.0:8080", ":8080", "192.168.0.10:80", "[::]:80"} {
		if err := CheckBind(a, ""); err == nil {
			t.Errorf("%s 는 토큰 없이 거부해야 함", a)
		}
		if err := CheckBind(a, "tok"); err != nil {
			t.Errorf("%s 토큰 있으면 허용: %v", a, err)
		}
	}
	s := NewServer(Options{Version: "test", Logger: discard()})
	if err := s.RunHTTP(context.Background(), "0.0.0.0:0", HTTPOptions{}); err == nil {
		t.Error("RunHTTP 는 비 loopback + 토큰 없음이면 시작 거부")
	}
}

func TestHTTPOrigin(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: discard()})
	_, c := newHTTPTest(t, s, HTTPOptions{AllowedOrigins: ParseOrigins("https://app.example.kr/, https://b.example")})
	cases := map[string]int{
		"https://evil.example":    403,
		"http://localhost.evil":   403,
		"null":                    403,
		"http://localhost:5173":   200,
		"http://127.0.0.1:3000":   200,
		"https://app.example.kr":  200,
		"https://b.example":       200,
		"https://app.example.kr2": 403,
	}
	for origin, want := range cases {
		r := c.do("POST", initBody, map[string]string{"Origin": origin})
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("Origin %s: %d, want %d", origin, r.StatusCode, want)
		}
	}
}

func TestHTTPHeaderKeysOverrideDefault(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	c.initialize()
	resp := c.do("POST", callLine(2, "sgis_admin_codes", `{}`, ""), map[string]string{headerAPIKeys: reqKey + ":" + reqSecret})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	if text, isErr := resultText(t, m); isErr || !strings.Contains(text, "서울특별시") {
		t.Fatalf("성공해야 함: %q", text)
	}
	if len(f.authKeys) != 1 || f.authKeys[0] != reqKey {
		t.Errorf("X-Api-Keys 키만 써야 함: %v", f.authKeys)
	}
	assertNoSecrets(t, string(b), reqKey, reqSecret, defKey, defSecret, "TOKEN-"+reqKey+"-zzzz")

	// 잘못된 머리글 키는 기본 키로 넘어가지 않음
	resp = c.do("POST", callLine(3, "sgis_admin_codes", `{}`, ""), map[string]string{headerAPIKeys: badKey + ":" + badSecret})
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(b, &m)
	if _, isErr := resultText(t, m); !isErr {
		t.Error("나쁜 머리글 키는 오류여야 함")
	}
	for _, k := range f.authKeys {
		if k == defKey {
			t.Error("기본 키로 넘어가면 안 됨")
		}
	}
	assertNoSecrets(t, string(b), badKey, badSecret)
}

func TestHTTPProgressBeforeResponseSSE(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	c.initialize()
	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"sgis_admin_codes","arguments":{"cd":"11"},"_meta":{"progressToken":"p-1"}}}`
	resp := c.do("POST", body, nil)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("progressToken + Accept SSE 면 SSE: %s", ct)
	}
	evs := readSSE(t, resp)
	if len(evs) < 3 {
		t.Fatalf("진행 2개 이상 + 응답: %v", evs)
	}
	last := evs[len(evs)-1]
	if fmt.Sprint(last["id"]) != "7" || last["result"] == nil {
		t.Errorf("마지막 이벤트는 응답: %v", last)
	}
	prev := 0.0
	for _, e := range evs[:len(evs)-1] {
		if e["method"] != "notifications/progress" {
			t.Fatalf("응답 전 이벤트는 진행 알림: %v", e)
		}
		p := e["params"].(map[string]interface{})
		if p["progressToken"] != "p-1" || p["progress"].(float64) <= prev {
			t.Errorf("진행 알림 이상: %v", p)
		}
		prev = p["progress"].(float64)
	}
	if _, ok := evs[len(evs)-2]["params"].(map[string]interface{})["total"]; !ok {
		t.Error("마지막 진행 알림에 total")
	}

	// progressToken 없으면 SSE·진행 알림 없음
	resp = c.do("POST", callLine(8, "sgis_admin_codes", `{}`, ""), nil)
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("progressToken 없으면 JSON: %s", ct)
	}
	resp.Body.Close()
}

func TestStdioProgress(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	_, out := rpc(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sgis_admin_codes","arguments":{},"_meta":{"progressToken":42}}}`,
		callLine(2, "sgis_years", `{}`, ""),
	)
	var progress, resp1 int
	for i, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(l, "notifications/progress") {
			if !strings.Contains(l, `"progressToken":42`) {
				t.Errorf("progressToken 이 그대로: %s", l)
			}
			if resp1 != 0 {
				t.Error("진행 알림은 응답보다 먼저")
			}
			progress++
		} else if strings.Contains(l, `"id":1`) {
			resp1 = i + 1
		}
	}
	if progress < 2 {
		t.Errorf("시작·끝 최소 2번: %d", progress)
	}
	if strings.Count(out, "notifications/progress") != progress {
		t.Error("progressToken 없는 요청(id 2)은 진행 알림 없음")
	}
}

func TestProgressThrottle(t *testing.T) {
	var got []map[string]interface{}
	p := newProgressReporter(json.RawMessage(`"x"`), func(n rpcNotification) {
		got = append(got, n.Params.(map[string]interface{}))
	})
	for i := 0; i < 20; i++ {
		p.report("중간", false)
	}
	p.report("끝", true)
	p.report("끝 뒤", false)
	if len(got) != 2 {
		t.Fatalf("1초 안의 중간 알림은 묶여야 함(시작+끝 2개): %d", len(got))
	}
	if got[1]["total"] != got[1]["progress"] {
		t.Errorf("끝 알림 total=progress: %v", got[1])
	}
}

func TestStdioCancelStopsWork(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	pr, pw := io.Pipe()
	var out safeBuf
	done := make(chan struct{})
	go func() {
		_ = s.Serve(context.Background(), pr, &out)
		close(done)
	}()
	fmt.Fprintln(pw, callLine(9, "sgis_admin_codes", `{"cd":"48"}`, ""))
	select {
	case <-f.started:
	case <-time.After(3 * time.Second):
		t.Fatal("상위 호출이 시작되지 않음")
	}
	fmt.Fprintln(pw, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":9,"reason":"user"}}`)
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled 를 받고도 상위 HTTP 호출이 멈추지 않음")
	}
	pw.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Serve 가 끝나지 않음")
	}
	if strings.Contains(out.String(), `"id":9`) {
		t.Errorf("취소된 요청에는 답하지 않음: %s", out.String())
	}
}

func TestHTTPCancelStopsWork(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	c.initialize()
	status := make(chan int, 1)
	go func() {
		r := c.do("POST", callLine(11, "sgis_admin_codes", `{"cd":"48"}`, ""), nil)
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if strings.Contains(string(b), `"id":11`) {
			status <- -1
			return
		}
		status <- r.StatusCode
	}()
	<-f.started
	r := c.do("POST", `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":11}}`, nil)
	r.Body.Close()
	if r.StatusCode != http.StatusAccepted {
		t.Errorf("cancelled 알림 POST 는 202: %d", r.StatusCode)
	}
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("상위 호출이 멈추지 않음")
	}
	if st := <-status; st != http.StatusAccepted {
		t.Errorf("취소된 요청에 응답이 오면 안 됨(202 빈 본문 기대): %d", st)
	}
}

func TestHTTPDisconnectCancels(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	c.initialize()
	ctx, cancel := context.WithCancel(context.Background())
	body := `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"sgis_admin_codes","arguments":{"cd":"48"},"_meta":{"progressToken":"d"}}}`
	req, _ := http.NewRequestWithContext(ctx, "POST", c.url, strings.NewReader(body))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(headerSession, c.session)
	go func() {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()
	<-f.started
	cancel() // SSE 연결 끊김
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("연결이 끊겨도 상위 호출이 멈추지 않음")
	}
}

func TestHTTPGetStreamPing(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: discard()})
	_, c := newHTTPTest(t, s, HTTPOptions{PingInterval: 30 * time.Millisecond})
	r := c.do("GET", "", map[string]string{"Accept": "text/event-stream"})
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("세션 없는 GET 은 400: %d", r.StatusCode)
	}
	c.initialize()
	r = c.do("GET", "", map[string]string{"Accept": "text/event-stream"})
	if r.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("GET 은 SSE: %s", r.Header.Get("Content-Type"))
	}
	br := bufio.NewReader(r.Body)
	line, _ := br.ReadString('\n')
	if !strings.HasPrefix(line, ": ping") {
		t.Errorf("ping 주석 기대: %q", line)
	}
	r.Body.Close()
}

func discard() *log.Logger { return log.New(io.Discard, "", 0) }

// safeBuf는 동시 쓰기·읽기에 안전한 버퍼입니다.
type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
