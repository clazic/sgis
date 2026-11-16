package mcp

import (
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

const (
	defKey    = "DEFAULTKEY0001aaaa"
	defSecret = "DEFAULTSECRET0001bbbb"
	reqKey    = "REQUESTKEY0002cccc"
	reqSecret = "REQUESTSECRET0002dddd"
	badKey    = "BADKEY0003eeee"
	badSecret = "BADSECRET0003ffff"
)

// fakeSGIS는 인증·통계 API 흉내 서버입니다. 키별로 다른 토큰을 주고, 받은 키·토큰을 기록합니다.
type fakeSGIS struct {
	mu        sync.Mutex
	authKeys  []string
	apiTokens []string
	apiCalls  int
	srv       *httptest.Server
}

func newFakeSGIS(t *testing.T) *fakeSGIS {
	f := &fakeSGIS{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/authentication.json"):
			k := q.Get("consumer_key")
			f.mu.Lock()
			f.authKeys = append(f.authKeys, k)
			f.mu.Unlock()
			if k == badKey {
				// 상위 오류 메시지에 키 원문이 섞여 오는 경우
				fmt.Fprintf(w, `{"errCd":-401,"errMsg":"invalid consumer_key=%s"}`, k)
				return
			}
			exp := time.Now().Add(4 * time.Hour).UnixMilli()
			fmt.Fprintf(w, `{"result":{"accessToken":"TOKEN-%s-zzzz","accessTimeout":"%d"},"errCd":0}`, k, exp)
		case strings.HasSuffix(r.URL.Path, "/addr/stage.json"):
			tok := q.Get("accessToken")
			f.mu.Lock()
			f.apiTokens = append(f.apiTokens, tok)
			f.apiCalls++
			f.mu.Unlock()
			if q.Get("cd") == "11" {
				time.Sleep(400 * time.Millisecond) // 느린 요청
			}
			if q.Get("cd") == "39" {
				// 토큰을 되비추는 오류 — 스크럽 확인용
				fmt.Fprintf(w, `{"errCd":-200,"errMsg":"bad request token=%s"}`, tok)
				return
			}
			fmt.Fprintf(w, `{"result":[{"cd":"11","addr_name":"서울특별시","tok":"%s"}],"errCd":0}`, tok)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSGIS) server(keys []Cred) *Server {
	return NewServer(Options{
		Version:     "test",
		DefaultKeys: keys,
		APIBaseURL:  f.srv.URL + "/",
		AuthURL:     f.srv.URL + "/auth/authentication.json",
		Logger:      log.New(io.Discard, "", 0),
	})
}

// rpc는 줄 단위 요청들을 보내고 id별 응답을 돌려줍니다.
func rpc(t *testing.T, s *Server, lines ...string) (map[string]map[string]interface{}, string) {
	t.Helper()
	var out bytes.Buffer
	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	if err := s.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	res := map[string]map[string]interface{}{}
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("stdout 에 JSON 아닌 줄: %q", l)
		}
		res[fmt.Sprint(m["id"])] = m
	}
	return res, out.String()
}

func callLine(id int, tool string, args string, meta string) string {
	if meta != "" {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s,"_meta":%s}}`, id, tool, args, meta)
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, tool, args)
}

func resultText(t *testing.T, m map[string]interface{}) (string, bool) {
	t.Helper()
	r, ok := m["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result 없음: %v", m)
	}
	content := r["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	isErr, _ := r["isError"].(bool)
	return text, isErr
}

func assertNoSecrets(t *testing.T, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("출력에 비밀 문자열이 그대로 있음: %s…", s[:4])
		}
	}
}

func TestScrub(t *testing.T) {
	text := "url=https://x/?consumer_key=AbC%2B123456789&consumer_secret=SECRETVALUE99 token TOKENTOKEN42 raw AbC+123456789"
	got := Scrub(text, []string{"AbC+123456789", "SECRETVALUE99", "TOKENTOKEN42", "", "short"})
	for _, s := range []string{"AbC+123456789", "AbC%2B123456789", "SECRETVALUE99", "TOKENTOKEN42"} {
		if strings.Contains(got, s) {
			t.Errorf("스크럽 안 됨: %q in %q", s, got)
		}
	}
	for _, want := range []string{"***6789", "***UE99", "***EN42"} {
		if !strings.Contains(got, want) {
			t.Errorf("마스크 %q 없음: %q", want, got)
		}
	}
	if Scrub("abc", nil) != "abc" {
		t.Error("비밀 없으면 그대로여야 함")
	}
	if maskSecret("1234567") != "***" {
		t.Error("짧은 값은 *** 만")
	}
}

func TestParseCred(t *testing.T) {
	c, err := ParseCred(" k1:s1:x ")
	if err != nil || c.Key != "k1" || c.Secret != "s1:x" {
		t.Fatalf("ParseCred: %+v %v", c, err)
	}
	for _, bad := range []string{"", "nokey", ":s", "k:", "k k:s"} {
		_, err := ParseCred(bad)
		if err == nil {
			t.Errorf("%q 는 오류여야 함", bad)
		} else if bad != "" && strings.Contains(err.Error(), bad) {
			t.Errorf("오류에 입력 원문이 들어감: %v", err)
		}
	}
	cs, err := ParseEnvKeys("a1:b1, ,a2:b2")
	if err != nil || len(cs) != 2 {
		t.Fatalf("ParseEnvKeys: %v %v", cs, err)
	}
}

func TestInitializeAndToolsListWithoutKeys(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: log.New(io.Discard, "", 0)})
	res, _ := rpc(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":4,"method":"nope"}`,
	)
	init := res["1"]["result"].(map[string]interface{})
	if init["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion: %v", init["protocolVersion"])
	}
	tools := res["2"]["result"].(map[string]interface{})["tools"].([]interface{})
	if len(tools) < 10 {
		t.Fatalf("도구 수가 너무 적음: %d", len(tools))
	}
	names := map[string]bool{}
	for _, tt := range tools {
		m := tt.(map[string]interface{})
		name := m["name"].(string)
		names[name] = true
		if !strings.HasPrefix(name, "sgis_") {
			t.Errorf("이름 규칙 위반: %s", name)
		}
		if m["description"].(string) == "" {
			t.Errorf("%s description 비어 있음", name)
		}
		sch := m["inputSchema"].(map[string]interface{})
		if sch["type"] != "object" || sch["additionalProperties"] != false {
			t.Errorf("%s inputSchema 이상: %v", name, sch)
		}
	}
	for _, want := range []string{"sgis_admin_codes", "sgis_population", "sgis_household", "sgis_house", "sgis_company", "sgis_geocode", "sgis_reverse_geocode", "sgis_boundary"} {
		if !names[want] {
			t.Errorf("도구 없음: %s", want)
		}
	}
	if _, ok := res["3"]["result"]; !ok {
		t.Error("ping 결과 없음")
	}
	if res["4"]["error"].(map[string]interface{})["code"].(float64) != -32601 {
		t.Error("모르는 메서드는 -32601")
	}
}

func TestOldProtocolEchoed(t *testing.T) {
	s := NewServer(Options{Version: "test", Logger: log.New(io.Discard, "", 0)})
	res, _ := rpc(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if res["1"]["result"].(map[string]interface{})["protocolVersion"] != "2024-11-05" {
		t.Error("옛 버전을 돌려줘야 함")
	}
}

func TestNoKeysIsError(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server(nil)
	res, _ := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, ""))
	text, isErr := resultText(t, res["1"])
	if !isErr || !strings.Contains(text, "API 키가 없어요") {
		t.Errorf("키 없음 오류여야 함: %v %q", isErr, text)
	}
	if f.apiCalls != 0 || len(f.authKeys) != 0 {
		t.Error("키가 없으면 네트워크를 쓰면 안 됨")
	}
}

func TestDefaultKeyUsedAndScrubbed(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, ""))
	text, isErr := resultText(t, res["1"])
	if isErr || !strings.Contains(text, "서울특별시") {
		t.Fatalf("성공해야 함: %q", text)
	}
	if len(f.authKeys) != 1 || f.authKeys[0] != defKey {
		t.Errorf("기본 키로 발급해야 함: %v", f.authKeys)
	}
	// 결과에 되비친 accessToken 도 가려져야 함
	assertNoSecrets(t, out, defKey, defSecret, "TOKEN-"+defKey+"-zzzz")
}

func TestRequestKeysOnly(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	meta := fmt.Sprintf(`{"kr.pes/keys":["%s:%s"]}`, reqKey, reqSecret)
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{"cd":"11"}`, meta))
	if _, isErr := resultText(t, res["1"]); isErr {
		t.Fatal("성공해야 함")
	}
	for _, k := range f.authKeys {
		if k != reqKey {
			t.Errorf("요청별 키가 있으면 기본 키를 쓰면 안 됨: %v", f.authKeys)
		}
	}
	for _, tok := range f.apiTokens {
		if tok != "TOKEN-"+reqKey+"-zzzz" {
			t.Errorf("요청별 키 토큰만 써야 함: %v", f.apiTokens)
		}
	}
	assertNoSecrets(t, out, reqKey, reqSecret, defKey, defSecret, "TOKEN-"+reqKey+"-zzzz")
}

func TestRequestKeysNeverFallBackToDefault(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	meta := fmt.Sprintf(`{"kr.pes/keys":["%s:%s"]}`, badKey, badSecret)
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, meta))
	text, isErr := resultText(t, res["1"])
	if !isErr {
		t.Fatalf("잘못된 요청별 키면 오류여야 함: %q", text)
	}
	for _, k := range f.authKeys {
		if k == defKey {
			t.Error("요청별 키 실패 시 기본 키로 넘어가면 안 됨")
		}
	}
	// 상위 오류 메시지에 섞여 온 키 원문도 가려져야 함
	assertNoSecrets(t, out, badKey, badSecret, defKey, defSecret)
	if !strings.Contains(text, maskSecret(badKey)) {
		t.Errorf("마스크된 키 표시가 있어야 함: %q", text)
	}
}

func TestEmptyOrMalformedMetaKeys(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	res, out := rpc(t, s,
		callLine(1, "sgis_admin_codes", `{}`, `{"kr.pes/keys":[]}`),
		callLine(2, "sgis_admin_codes", `{}`, `{"kr.pes/keys":["NOCOLONSECRETVALUE123"]}`),
		callLine(3, "sgis_admin_codes", `{}`, `{"kr.pes/keys":"x"}`),
	)
	for _, id := range []string{"1", "2", "3"} {
		if _, isErr := resultText(t, res[id]); !isErr {
			t.Errorf("id %s 는 오류여야 함", id)
		}
	}
	if len(f.authKeys) != 0 {
		t.Error("기본 키로 넘어가면 안 됨")
	}
	assertNoSecrets(t, out, "NOCOLONSECRETVALUE123")
}

func TestKeyPoolSkipsBadKey(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{badKey, badSecret}, {defKey, defSecret}})
	var out string
	for id := 1; id <= 3; id++ { // 순차 호출: 라운드로빈이 돌아도 쿨다운 중인 나쁜 키는 뒤로 밀림
		res, o := rpc(t, s, callLine(id, "sgis_admin_codes", `{}`, ""))
		out += o
		if text, isErr := resultText(t, res[fmt.Sprint(id)]); isErr {
			t.Errorf("id %d: 나쁜 키를 건너뛰고 성공해야 함: %q", id, text)
		}
	}
	bad := 0
	for _, k := range f.authKeys {
		if k == badKey {
			bad++
		}
	}
	if bad != 1 {
		t.Errorf("나쁜 키는 쿨다운으로 한 번만 시도해야 함: %d", bad)
	}
	assertNoSecrets(t, out, badKey, badSecret, defKey, defSecret)
}

func TestTokensSeparatePerKey(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server(nil)
	m1 := fmt.Sprintf(`{"kr.pes/keys":["%s:%s"]}`, reqKey, reqSecret)
	m2 := fmt.Sprintf(`{"kr.pes/keys":["%s:%s"]}`, defKey, defSecret)
	rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, m1))
	rpc(t, s, callLine(2, "sgis_admin_codes", `{}`, m2))
	rpc(t, s, callLine(3, "sgis_admin_codes", `{}`, m1))
	want := []string{"TOKEN-" + reqKey + "-zzzz", "TOKEN-" + defKey + "-zzzz", "TOKEN-" + reqKey + "-zzzz"}
	if fmt.Sprint(f.apiTokens) != fmt.Sprint(want) {
		t.Errorf("키별 토큰 분리 실패: %v", f.apiTokens)
	}
	if len(f.authKeys) != 2 {
		t.Errorf("같은 키는 토큰 재사용(발급 2회): %v", f.authKeys)
	}
}

func TestUpstreamErrorScrubbed(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{"cd":"39"}`, ""))
	if _, isErr := resultText(t, res["1"]); !isErr {
		t.Fatal("오류여야 함")
	}
	assertNoSecrets(t, out, "TOKEN-"+defKey+"-zzzz", defKey)
}

func TestInputValidation(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	cases := []string{
		callLine(1, "sgis_admin_codes", `{"cd":"1"}`, ""),
		callLine(2, "sgis_population", `{"adm_cd":"11"}`, ""),
		callLine(3, "sgis_population", `{"year":"20x0"}`, ""),
		callLine(4, "sgis_population", `{"year":"2020","adm_cd":"1101"}`, ""),
		callLine(5, "sgis_population", `{"year":"2020","low_search":"9"}`, ""),
		callLine(6, "sgis_geocode", `{"address":"`+strings.Repeat("가", 201)+`"}`, ""),
		callLine(7, "sgis_population", `{"year":"2020","bogus":1}`, ""),
		callLine(8, "sgis_reverse_geocode", `{"x":0,"y":0}`, ""),
		callLine(9, "sgis_nope", `{}`, ""),
		callLine(10, "sgis_boundary", `{"year":"2024"}`, ""),
	}
	res, _ := rpc(t, s, cases...)
	for i := 1; i <= len(cases); i++ {
		text, isErr := resultText(t, res[fmt.Sprint(i)])
		if !isErr {
			t.Errorf("case %d 는 오류여야 함: %q", i, text)
		}
	}
	if f.apiCalls != 0 {
		t.Errorf("검증 실패면 API 호출 없어야 함: %d", f.apiCalls)
	}
	// 정수 연도는 받아 준다
	v, err := s.toolByName["sgis_population"].validate(map[string]interface{}{"year": float64(2020)})
	if err != nil || v["year"] != "2020" {
		t.Errorf("정수 연도: %v %v", v, err)
	}
}

func TestConcurrentCallsDoNotBlock(t *testing.T) {
	f := newFakeSGIS(t)
	s := f.server([]Cred{{defKey, defSecret}})
	_, out := rpc(t, s,
		callLine(1, "sgis_admin_codes", `{"cd":"11"}`, ""), // 느림
		callLine(2, "sgis_admin_codes", `{"cd":"26"}`, ""),
	)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":2`) {
		t.Errorf("빠른 요청이 먼저 답해야 함: %v", lines)
	}
}

func TestBoundaryTextTruncatesByFeature(t *testing.T) {
	var feats []string
	for i := 0; i < 50; i++ {
		feats = append(feats, fmt.Sprintf(`{"type":"Feature","properties":{"adm_cd":"%d"},"geometry":{"type":"Point","coordinates":[1000000.0,2000000.0]}}`, 11000+i))
	}
	raw := json.RawMessage(`{"type":"FeatureCollection","features":[` + strings.Join(feats, ",") + `]}`)
	full, err := boundaryText(raw, true, MaxResultBytes)
	if err != nil {
		t.Fatal(err)
	}
	var fc map[string]interface{}
	if err := json.Unmarshal([]byte(full), &fc); err != nil {
		t.Fatal(err)
	}
	c := fc["features"].([]interface{})[0].(map[string]interface{})["geometry"].(map[string]interface{})["coordinates"].([]interface{})
	if c[0].(float64) != 127.5 || c[1].(float64) != 38 {
		t.Errorf("WGS84 재투영·반올림: %v", c)
	}
	small, err := boundaryText(raw, false, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(small) > 2000 {
		t.Errorf("상한 초과: %d", len(small))
	}
	if err := json.Unmarshal([]byte(small), &fc); err != nil {
		t.Fatalf("잘린 결과도 유효한 JSON 이어야 함: %v", err)
	}
	if !strings.Contains(fc["sgis_mcp_note"].(string), "잘림") {
		t.Error("잘림 안내 없음")
	}
	if n := len(fc["features"].([]interface{})); n == 0 || n >= 50 {
		t.Errorf("일부 feature 만 담아야 함: %d", n)
	}
}

func TestCapText(t *testing.T) {
	s := strings.Repeat("가", 1000)
	got := capText(s, 500)
	if len(got) > 500 || !strings.Contains(got, "잘림") || !json.Valid([]byte(`"`+got[:10]+`"`)) {
		t.Errorf("capText: len=%d", len(got))
	}
}
