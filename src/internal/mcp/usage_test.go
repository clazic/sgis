package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	limKey    = "LIMITEDKEY0004gggg"
	limSecret = "LIMITEDSECRET0004hhhh"
)

func reqMeta(pairs ...string) string {
	b, _ := json.Marshal(map[string]interface{}{metaKeysField: pairs})
	return string(b)
}

// usageOf는 응답 result._meta["kr.pes/key-usage"] 를 읽습니다. 없으면 (nil, false).
// 항목에 i · calls · errors · limited 말고 다른 칸이 있으면 실패합니다.
func usageOf(t *testing.T, m map[string]interface{}) ([]KeyUsage, bool) {
	t.Helper()
	r, ok := m["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result 없음: %v", m)
	}
	meta, ok := r["_meta"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	raw, ok := meta[metaKeyUsageField].([]interface{})
	if !ok {
		return nil, false
	}
	var out []KeyUsage
	for _, it := range raw {
		e := it.(map[string]interface{})
		if len(e) != 4 {
			t.Errorf("사용량 항목은 i·calls·errors·limited 네 칸이어야 함: %v", e)
		}
		for k, v := range e {
			if _, isNum := v.(float64); !isNum {
				t.Errorf("사용량 칸 %s 는 숫자여야 함: %v", k, v)
			}
		}
		out = append(out, KeyUsage{I: int(e["i"].(float64)), Calls: int(e["calls"].(float64)),
			Errors: int(e["errors"].(float64)), Limited: int(e["limited"].(float64))})
	}
	return out, true
}

func wantUsage(t *testing.T, m map[string]interface{}, want ...KeyUsage) {
	t.Helper()
	got, ok := usageOf(t, m)
	if !ok {
		t.Fatalf("kr.pes/key-usage 가 있어야 함: %v", m["result"])
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("사용량 = %+v, want %+v", got, want)
	}
}

func TestKeyUsageAuthAndDataCalls(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), []Cred{{defKey, defSecret}})
	meta := reqMeta(reqKey + ":" + reqSecret)

	// 첫 호출: 인증 1 + 자료 1
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, meta))
	if _, isErr := resultText(t, res["1"]); isErr {
		t.Fatal("성공해야 함")
	}
	wantUsage(t, res["1"], KeyUsage{I: 0, Calls: 2})
	if !strings.Contains(out, `"_meta":{"kr.pes/key-usage":[{"i":0,"calls":2,"errors":0,"limited":0}]}`) {
		t.Errorf("결과 꼴이 다름: %s", out)
	}
	assertNoSecrets(t, out, reqKey, reqSecret, "TOKEN-"+reqKey+"-zzzz")

	// 둘째 호출: 토큰 캐시 → 자료 1 만
	res, _ = rpc(t, s, callLine(2, "sgis_admin_codes", `{}`, meta))
	wantUsage(t, res["2"], KeyUsage{I: 0, Calls: 1})
}

func TestKeyUsageAbsentForDefaultKeys(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), []Cred{{defKey, defSecret}})
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, ""))
	if _, isErr := resultText(t, res["1"]); isErr {
		t.Fatal("성공해야 함")
	}
	if f.apiCalls != 1 {
		t.Fatalf("SGIS 를 불렀어야 함: %d", f.apiCalls)
	}
	if _, ok := usageOf(t, res["1"]); ok || strings.Contains(out, "_meta") || strings.Contains(out, metaKeyUsageField) {
		t.Errorf("기본 키 요청에는 사용량 칸이 없어야 함: %s", out)
	}
}

func TestKeyUsageBadKeyThenGoodKey(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), nil)
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, reqMeta(badKey+":"+badSecret, reqKey+":"+reqSecret)))
	if _, isErr := resultText(t, res["1"]); isErr {
		t.Fatal("둘째 키로 성공해야 함")
	}
	// 0번: 인증 요청 1(errCd=-401), 1번: 인증 1 + 자료 1
	wantUsage(t, res["1"], KeyUsage{I: 0, Calls: 1, Errors: 1}, KeyUsage{I: 1, Calls: 2})
	assertNoSecrets(t, out, badKey, badSecret, reqKey, reqSecret)
}

func TestKeyUsageOnErrorResult(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), nil)

	// 자료 요청이 errCd != 0 → isError 결과에도 사용량
	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{"cd":"39"}`, reqMeta(reqKey+":"+reqSecret)))
	if _, isErr := resultText(t, res["1"]); !isErr {
		t.Fatal("오류여야 함")
	}
	wantUsage(t, res["1"], KeyUsage{I: 0, Calls: 2, Errors: 1})
	assertNoSecrets(t, out, reqKey, reqSecret, "TOKEN-"+reqKey+"-zzzz")

	// 모든 키가 인증 실패
	res, out = rpc(t, s, callLine(2, "sgis_admin_codes", `{}`, reqMeta(badKey+":"+badSecret)))
	if _, isErr := resultText(t, res["2"]); !isErr {
		t.Fatal("오류여야 함")
	}
	wantUsage(t, res["2"], KeyUsage{I: 0, Calls: 1, Errors: 1})
	assertNoSecrets(t, out, badKey, badSecret)

	// 입력 오류(상위 요청 없음) → 칸 없음
	res, _ = rpc(t, s, callLine(3, "sgis_admin_codes", `{"cd":"x"}`, reqMeta(reqKey+":"+reqSecret)))
	if _, isErr := resultText(t, res["3"]); !isErr {
		t.Fatal("입력 오류여야 함")
	}
	if _, ok := usageOf(t, res["3"]); ok {
		t.Error("상위 요청이 없으면 사용량 칸이 없어야 함")
	}
}

// TestKeyUsageRateLimited는 429 가 limited(그리고 errors)로 세어지는지 봅니다.
// api.Client 가 429 를 1·2·4초 쉬며 다시 보내므로 7초쯤 걸립니다.
func TestKeyUsageRateLimited(t *testing.T) {
	if testing.Short() {
		t.Skip("429 재시도 대기(약 7초)")
	}
	var mu sync.Mutex
	dataHits := map[string]int{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if strings.HasSuffix(r.URL.Path, "/auth/authentication.json") {
			exp := time.Now().Add(4 * time.Hour).UnixMilli()
			fmt.Fprintf(w, `{"result":{"accessToken":"TOKEN-%s-zzzz","accessTimeout":"%d"},"errCd":0}`, q.Get("consumer_key"), exp)
			return
		}
		tok := q.Get("accessToken")
		mu.Lock()
		dataHits[tok]++
		mu.Unlock()
		if strings.Contains(tok, limKey) {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"result":[{"cd":"11","addr_name":"서울특별시"}],"errCd":0}`)
	}))
	defer up.Close()
	s := NewServer(Options{Version: "test", Results: newStore(t), Logger: discard(),
		APIBaseURL: up.URL + "/", AuthURL: up.URL + "/auth/authentication.json"})

	res, out := rpc(t, s, callLine(1, "sgis_admin_codes", `{}`, reqMeta(limKey+":"+limSecret, reqKey+":"+reqSecret)))
	if _, isErr := resultText(t, res["1"]); isErr {
		t.Fatal("둘째 키로 성공해야 함")
	}
	// 0번: 인증 1 + 자료 4(처음 + 재시도 3) 모두 429, 1번: 인증 1 + 자료 1
	wantUsage(t, res["1"], KeyUsage{I: 0, Calls: 5, Errors: 4, Limited: 4}, KeyUsage{I: 1, Calls: 2})
	if n := dataHits["TOKEN-"+limKey+"-zzzz"]; n != 4 {
		t.Errorf("상위가 받은 429 요청 수 = %d, want 4", n)
	}
	assertNoSecrets(t, out, limKey, limSecret, reqKey, reqSecret)
}

func TestKeyUsageLargeResultAndPaging(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), nil)
	args := `{"year":"2020","adm_cd":"11","low_search":"2"}`
	res, out := rpc(t, s, callLine(1, "sgis_population", args, reqMeta(reqKey+":"+reqSecret)))
	text, isErr := resultText(t, res["1"])
	if isErr {
		t.Fatalf("오류: %s", text)
	}
	h := headOf(t, text, "columns")
	cur, _ := h["next_cursor"].(string)
	if cur == "" {
		t.Fatalf("큰 결과(쪽 나눔)여야 함: %v", h)
	}
	wantUsage(t, res["1"], KeyUsage{I: 0, Calls: 2})
	assertNoSecrets(t, out, reqKey, reqSecret)

	// 저장된 결과의 다음 쪽: 상위 요청이 없으므로 칸 없음(요청별 키를 함께 보내도)
	b, _ := json.Marshal(map[string]interface{}{"cursor": cur})
	res, _ = rpc(t, s, callLine(2, "sgis_result_page", string(b), reqMeta(reqKey+":"+reqSecret)))
	if ptext, isErr := resultText(t, res["2"]); isErr {
		t.Fatalf("다음 쪽 오류: %s", ptext)
	}
	if _, ok := usageOf(t, res["2"]); ok {
		t.Error("저장된 결과 쪽 넘김에는 사용량 칸이 없어야 함")
	}

	// GeoJSON 큰 결과 경로
	res, _ = rpc(t, s, callLine(3, "sgis_boundary", `{"year":"2020","adm_cd":"11","low_search":"1"}`, reqMeta(reqKey+":"+reqSecret)))
	if gtext, isErr := resultText(t, res["3"]); isErr {
		t.Fatalf("경계 오류: %s", gtext)
	}
	wantUsage(t, res["3"], KeyUsage{I: 0, Calls: 1}) // 토큰 캐시 → 자료 1
}

func TestKeyUsageHTTPHeaderKeysAndSSE(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), []Cred{{defKey, defSecret}})
	_, c := newHTTPTest(t, s, HTTPOptions{})
	c.initialize()
	hdr := map[string]string{headerAPIKeys: badKey + ":" + badSecret + "," + reqKey + ":" + reqSecret}

	// JSON 응답 + X-Api-Keys
	resp := c.do("POST", callLine(2, "sgis_admin_codes", `{}`, ""), hdr)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON 아님: %s", b)
	}
	if _, isErr := resultText(t, m); isErr {
		t.Fatal("성공해야 함")
	}
	wantUsage(t, m, KeyUsage{I: 0, Calls: 1, Errors: 1}, KeyUsage{I: 1, Calls: 2})
	assertNoSecrets(t, string(b), badKey, badSecret, reqKey, reqSecret)

	// SSE 로 오는 결과(progressToken) + _meta 키
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"sgis_admin_codes","arguments":{},"_meta":{"progressToken":"p-1","kr.pes/keys":["%s:%s"]}}}`, reqKey, reqSecret)
	resp = c.do("POST", body, nil)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("SSE 여야 함: %s", ct)
	}
	evs := readSSE(t, resp)
	last := evs[len(evs)-1]
	if fmt.Sprint(last["id"]) != "7" {
		t.Fatalf("마지막 이벤트는 응답: %v", last)
	}
	wantUsage(t, last, KeyUsage{I: 0, Calls: 1}) // 토큰 캐시 → 자료 1

	// 기본 키(머리글 · _meta 없음) → 칸 없음
	resp = c.do("POST", callLine(8, "sgis_admin_codes", `{}`, ""), nil)
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(b, &m)
	if _, ok := usageOf(t, m); ok || strings.Contains(string(b), "_meta") {
		t.Errorf("기본 키 요청에는 사용량 칸이 없어야 함: %s", b)
	}
}

// TestUsageTransport는 전송 계층 집계 규칙(HTTP 400 이상 · 전송 오류 · errCd · slot 없음)을 봅니다.
func TestUsageTransport(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/500":
			w.WriteHeader(http.StatusInternalServerError)
		case "/429":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/errcd":
			io.WriteString(w, `{"errCd":-200,"errMsg":"x"}`)
		case "/big":
			io.WriteString(w, `{"result":"`+strings.Repeat("a", usageSniffBytes)+`","errCd":0}`)
		default:
			io.WriteString(w, `{"result":[],"errCd":0}`)
		}
	}))
	hc := &http.Client{Transport: wrapUsageTransport(nil)}
	u := newKeyUsage()
	get := func(ctx context.Context, url string) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := hc.Do(req)
		if err == nil {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ { // 동시 집계
		wg.Add(1)
		go func() {
			defer wg.Done()
			get(withUsage(context.Background(), u, 3), up.URL+"/ok")
		}()
	}
	wg.Wait()
	get(withUsage(context.Background(), u, 3), up.URL+"/big")
	get(withUsage(context.Background(), u, 1), up.URL+"/500")
	get(withUsage(context.Background(), u, 1), up.URL+"/429")
	get(withUsage(context.Background(), u, 1), up.URL+"/errcd")
	get(context.Background(), up.URL+"/500") // slot 없음 → 세지 않음
	up.Close()
	get(withUsage(context.Background(), u, 2), up.URL+"/ok") // 전송 오류

	want := []KeyUsage{{I: 1, Calls: 3, Errors: 3, Limited: 1}, {I: 2, Calls: 1, Errors: 1}, {I: 3, Calls: 9}}
	if got := u.entries(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("집계 = %+v, want %+v", got, want)
	}
	if (*keyUsage)(nil).entries() != nil || newKeyUsage().entries() != nil {
		t.Error("집계가 없으면 nil")
	}
}
