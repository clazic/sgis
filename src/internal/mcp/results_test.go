package mcp

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testPageBytes = 6000

func newStore(t *testing.T) *ResultStore {
	return &ResultStore{Dir: t.TempDir(), TTL: time.Hour, MaxBytes: 1 << 30}
}

func pagedServer(f *fakeSGIS, rs *ResultStore, keys []Cred) *Server {
	return NewServer(Options{
		Version: "test", DefaultKeys: keys, Results: rs, PageBytes: testPageBytes,
		APIBaseURL: f.srv.URL + "/", AuthURL: f.srv.URL + "/auth/authentication.json", Logger: discard(),
	})
}

// call은 도구 하나를 부르고 (텍스트, 오류여부, content 전체)를 돌려줍니다.
func call(t *testing.T, s *Server, tool string, args interface{}) (string, bool, []interface{}) {
	t.Helper()
	b, _ := json.Marshal(args)
	res, _ := rpc(t, s, callLine(1, tool, string(b), ""))
	r := res["1"]["result"].(map[string]interface{})
	content := r["content"].([]interface{})
	isErr, _ := r["isError"].(bool)
	return content[0].(map[string]interface{})["text"].(string), isErr, content
}

func headOf(t *testing.T, text, format string) map[string]interface{} {
	t.Helper()
	js := text
	if format == "csv" {
		js = strings.SplitN(text, "\n", 2)[0]
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("요약 JSON 아님: %v\n%.300s", err, text)
	}
	return m
}

// pageRows는 한 쪽에서 (adm_cd, adm_nm, tot_ppltn) 을 형식별로 되살립니다.
func pageRows(t *testing.T, text, format string) [][3]string {
	t.Helper()
	var out [][3]string
	switch format {
	case "csv":
		r := csv.NewReader(strings.NewReader(strings.SplitN(text, "\n", 2)[1]))
		recs, err := r.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		hdr := recs[0]
		ci, ni, pi := indexOf(hdr, "adm_cd"), indexOf(hdr, "adm_nm"), indexOf(hdr, "tot_ppltn")
		for _, rec := range recs[1:] {
			out = append(out, [3]string{rec[ci], rec[ni], rec[pi]})
		}
	case "json":
		var m struct {
			Data []map[string]string `json:"data"`
		}
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Fatal(err)
		}
		for _, d := range m.Data {
			out = append(out, [3]string{d["adm_cd"], d["adm_nm"], d["tot_ppltn"]})
		}
	case "columns":
		var m struct {
			Columns     []string                     `json:"columns"`
			Rows        [][]string                   `json:"rows"`
			Dict        map[string]map[string]string `json:"dict"`
			DictColumns map[string]string            `json:"dict_columns"`
		}
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Fatal(err)
		}
		if indexOf(m.Columns, "adm_nm") >= 0 || m.DictColumns["adm_cd"] != "adm_nm" {
			t.Fatalf("columns 형식은 adm_nm 을 dict 로 보내야 함: %v %v", m.Columns, m.DictColumns)
		}
		ci, pi := indexOf(m.Columns, "adm_cd"), indexOf(m.Columns, "tot_ppltn")
		for _, r := range m.Rows {
			out = append(out, [3]string{r[ci], m.Dict["adm_cd"][r[ci]], r[pi]})
		}
	}
	return out
}

func expectedRow(i int) [3]string {
	pop := fmt.Sprint(1000 + i)
	if i == fakePopRows-1 {
		pop = "N/A"
	}
	return [3]string{fmt.Sprint(11010500 + i), fmt.Sprintf("서울특별시 테스트구 동%03d", i), pop}
}

func TestPagingRoundTripAllFormats(t *testing.T) {
	for _, format := range []string{"columns", "csv", "json", ""} {
		t.Run("format="+format, func(t *testing.T) {
			f := newFakeSGIS(t)
			rs := newStore(t)
			s := pagedServer(f, rs, []Cred{{defKey, defSecret}})
			args := map[string]interface{}{"year": "2020", "adm_cd": "11", "low_search": "2"}
			want := format
			if format != "" {
				args["format"] = format
			} else {
				want = "columns" // 큰 결과 기본
			}
			text, isErr, content := call(t, s, "sgis_population", args)
			if isErr {
				t.Fatalf("오류: %s", text)
			}
			if len(text) > testPageBytes {
				t.Errorf("첫 쪽이 상한을 넘음: %d", len(text))
			}
			h := headOf(t, text, want)
			if h["format"] != want || h["truncated"] != true || h["next_cursor"] == nil || h["result_id"] == nil || int(h["total_rows"].(float64)) != fakePopRows {
				t.Fatalf("요약 칸 이상: %v", h)
			}
			if !strings.HasPrefix(text, `{"total_rows":`) {
				t.Error("요약 칸이 맨 앞이어야 함")
			}
			// resource_link
			if len(content) != 2 {
				t.Fatalf("resource_link 가 붙어야 함: %v", content)
			}
			link := content[1].(map[string]interface{})
			id := h["result_id"].(string)
			if link["type"] != "resource_link" || link["uri"] != "sgis://results/"+id+".csv" || link["name"] != id+".csv" ||
				link["mimeType"] != "text/csv" || link["size"].(float64) <= 0 || link["description"] != fmt.Sprintf("전체 %d행", fakePopRows) {
				t.Errorf("resource_link 모양: %v", link)
			}

			rows := pageRows(t, text, want)
			// 다음 쪽은 키 없는 서버(같은 저장소)에서 — result_page 는 키 불필요
			keyless := pagedServer(f, rs, nil)
			cur := h["next_cursor"].(string)
			pages := 1
			for cur != "" {
				ptext, isErr, _ := call(t, keyless, "sgis_result_page", map[string]interface{}{"cursor": cur})
				if isErr {
					t.Fatalf("result_page 오류: %s", ptext)
				}
				if len(ptext) > testPageBytes {
					t.Errorf("쪽이 상한을 넘음: %d", len(ptext))
				}
				ph := headOf(t, ptext, want)
				if ph["format"] != want {
					t.Errorf("같은 형식이어야 함: %v", ph["format"])
				}
				rows = append(rows, pageRows(t, ptext, want)...)
				cur, _ = ph["next_cursor"].(string)
				if cur == "" && ph["truncated"] != false {
					t.Error("마지막 쪽은 truncated=false")
				}
				pages++
			}
			if pages < 3 {
				t.Errorf("여러 쪽이어야 함: %d", pages)
			}
			if len(rows) != fakePopRows {
				t.Fatalf("모은 행 수 %d, 원래 %d", len(rows), fakePopRows)
			}
			for i, r := range rows {
				if r != expectedRow(i) {
					t.Fatalf("%d번째 행 다름: %v vs %v", i, r, expectedRow(i))
				}
			}
			if f.apiCalls != 1 {
				t.Errorf("다음 쪽은 다시 조회하지 않음: SGIS 호출 %d번", f.apiCalls)
			}
			// limit
			ltext, _, _ := call(t, keyless, "sgis_result_page", map[string]interface{}{"cursor": h["next_cursor"], "limit": 3})
			if n := len(pageRows(t, ltext, want)); n != 3 {
				t.Errorf("limit 3: %d", n)
			}
		})
	}
}

func TestSmallResultUnchanged(t *testing.T) {
	f := newFakeSGIS(t)
	s := pagedServer(f, newStore(t), []Cred{{defKey, defSecret}})
	s.pageBytes = MaxResultBytes
	text, isErr, content := call(t, s, "sgis_population", map[string]interface{}{"year": "2020", "adm_cd": "11"})
	if isErr || !strings.HasPrefix(text, `[{"adm_cd":`) || len(content) != 1 {
		t.Errorf("작은 결과는 원본 JSON 배열 그대로(호환): %.80s", text)
	}
}

func TestFilterAggregateExact(t *testing.T) {
	raw := json.RawMessage(`[
	 {"adm_cd":"11","adm_nm":"서울","year":"2020","pop":"100"},
	 {"adm_cd":"11","adm_nm":"서울","year":"2021","pop":"110"},
	 {"adm_cd":"26","adm_nm":"부산","year":"2020","pop":"50"},
	 {"adm_cd":"26","adm_nm":"부산","year":"2021","pop":"N/A"},
	 {"adm_cd":"27","adm_nm":"대구","year":"2021","pop":"30"}]`)
	tb, ok := decodeTable(raw)
	if !ok || strings.Join(tb.Columns, ",") != "adm_cd,adm_nm,year,pop" {
		t.Fatalf("decodeTable: %v", tb)
	}
	ft, err := applyFilter(tb, map[string]interface{}{"adm_cd": []interface{}{"11", float64(26)}})
	if err != nil || len(ft.Rows) != 4 {
		t.Fatalf("목록 filter: %v %v", ft, err)
	}
	ft, _ = applyFilter(tb, map[string]interface{}{"pop": map[string]interface{}{"from": float64(40), "to": "105"}})
	if len(ft.Rows) != 2 { // 100, 50 (N/A 는 문자열 비교로 범위 밖)
		t.Errorf("범위 filter: %v", ft.Rows)
	}
	ft, _ = applyFilter(tb, map[string]interface{}{"year": "2021"})
	if len(ft.Rows) != 3 {
		t.Errorf("같음 filter: %v", ft.Rows)
	}
	if _, err := applyFilter(tb, map[string]interface{}{"nope": "1"}); err == nil {
		t.Error("없는 열은 오류")
	}

	check := func(op string, want map[string]float64, wantSkip int) {
		spec, err := parseAgg(map[string]interface{}{"group_by": []interface{}{"adm_cd"}, "op": op, "value": "pop"})
		if err != nil {
			t.Fatal(err)
		}
		at, skipped, err := applyAggregate(tb, spec)
		if err != nil {
			t.Fatal(err)
		}
		if skipped != wantSkip {
			t.Errorf("%s 건너뜀 %d, want %d", op, skipped, wantSkip)
		}
		for _, r := range at.Rows {
			code := r[0].(string)
			var got float64
			switch x := r[1].(type) {
			case float64:
				got = x
			case int:
				got = float64(x)
			}
			if got != want[code] {
				t.Errorf("%s[%s] = %v, want %v", op, code, got, want[code])
			}
		}
	}
	check("sum", map[string]float64{"11": 210, "26": 50, "27": 30}, 1)
	check("avg", map[string]float64{"11": 105, "26": 50, "27": 30}, 1)
	check("min", map[string]float64{"11": 100, "26": 50, "27": 30}, 1)
	check("max", map[string]float64{"11": 110, "26": 50, "27": 30}, 1)
	check("count", map[string]float64{"11": 2, "26": 1, "27": 1}, 1)

	spec, _ := parseAgg(map[string]interface{}{"group_by": []interface{}{"year"}, "op": "count"})
	at, _, _ := applyAggregate(tb, spec)
	if fmt.Sprint(at.Rows) != "[[2020 2 2] [2021 3 3]]" {
		t.Errorf("value 없는 count: %v", at.Rows)
	}
	if _, err := parseAgg(map[string]interface{}{"op": "median", "value": "pop"}); err == nil {
		t.Error("모르는 op 는 오류")
	}
}

func TestToolFilterAggregateAndResultQuery(t *testing.T) {
	f := newFakeSGIS(t)
	rs := newStore(t)
	s := pagedServer(f, rs, []Cred{{defKey, defSecret}})
	base := map[string]interface{}{"year": "2020", "adm_cd": "11", "low_search": "2"}

	args := map[string]interface{}{"filter": map[string]interface{}{"adm_cd": []string{"11010500", "11010502"}}}
	for k, v := range base {
		args[k] = v
	}
	text, isErr, _ := call(t, s, "sgis_population", args)
	if isErr {
		t.Fatal(text)
	}
	h := headOf(t, text, "json")
	if h["format"] != "json" || h["total_rows"].(float64) != 2 || h["truncated"] != false {
		t.Errorf("filter 결과: %v", h)
	}

	// 큰 결과를 저장한 뒤 result_query 로 시도별 합계 (키 없는 서버)
	text, _, _ = call(t, s, "sgis_population", base)
	id := headOf(t, text, "columns")["result_id"].(string)
	keyless := pagedServer(f, rs, nil)
	q, isErr, _ := call(t, keyless, "sgis_result_query", map[string]interface{}{
		"result_id": id, "aggregate": map[string]interface{}{"group_by": []string{"sido"}, "op": "sum", "value": "tot_ppltn"}})
	if isErr {
		t.Fatal(q)
	}
	var out struct {
		Data    []map[string]interface{} `json:"data"`
		Skipped float64                  `json:"skipped_non_numeric"`
	}
	_ = json.Unmarshal([]byte(q), &out)
	// 손계산: i=0..299 의 1000+i 를 짝수(sido 11)·홀수(sido 12)로 나눔, i=300(N/A) 건너뜀
	var want11, want12 float64
	for i := 0; i < fakePopRows-1; i++ {
		if i%2 == 0 {
			want11 += float64(1000 + i)
		} else {
			want12 += float64(1000 + i)
		}
	}
	got := map[string]float64{}
	for _, d := range out.Data {
		got[d["sido"].(string)] = d["sum_tot_ppltn"].(float64)
	}
	if got["11"] != want11 || got["12"] != want12 || out.Skipped != 1 {
		t.Errorf("result_query 합계: %v (skip %v), want 11=%v 12=%v", got, out.Skipped, want11, want12)
	}
	if f.apiCalls != 2 {
		t.Errorf("result_query 는 다시 조회하지 않음: %d", f.apiCalls)
	}
}

func TestResultExpired(t *testing.T) {
	f := newFakeSGIS(t)
	rs := newStore(t)
	s := pagedServer(f, rs, []Cred{{defKey, defSecret}})
	text, _, _ := call(t, s, "sgis_population", map[string]interface{}{"year": "2020", "adm_cd": "11", "low_search": "2"})
	h := headOf(t, text, "columns")
	rs.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	ptext, isErr, _ := call(t, s, "sgis_result_page", map[string]interface{}{"cursor": h["next_cursor"]})
	if !isErr || !strings.Contains(ptext, "결과가 만료됐어요 — 다시 조회하세요") {
		t.Errorf("만료 오류: %v %q", isErr, ptext)
	}
	_, isErr, _ = call(t, s, "sgis_result_page", map[string]interface{}{"cursor": "bm90LWEtY3Vyc29y"})
	if !isErr {
		t.Error("엉터리 cursor 는 오류")
	}
}

func TestCleanupTTLAndSize(t *testing.T) {
	rs := newStore(t)
	t0 := time.Now()
	tb := &table{Columns: []string{"a"}, Rows: [][]interface{}{{"x"}, {"y"}}}
	rs.now = func() time.Time { return t0 }
	m1, _ := rs.SaveTable("t", nil, tb)
	rs.now = func() time.Time { return t0.Add(time.Minute) }
	m2, _ := rs.SaveTable("t", nil, tb)
	rs.now = func() time.Time { return t0.Add(2 * time.Minute) }
	m3, _ := rs.SaveTable("t", nil, tb)
	_ = os.WriteFile(filepath.Join(rs.Dir, "unrelated.txt"), []byte("keep"), 0o644)

	// 용량: 결과 하나 크기만큼만 허용 → 가장 새 것만 남음
	one := int64(0)
	for _, e := range []string{".jsonl", ".csv", ".meta.json"} {
		st, _ := os.Stat(filepath.Join(rs.Dir, m3.ID+e))
		one += st.Size()
	}
	rs.MaxBytes = one
	rs.Cleanup()
	if _, err := rs.Meta(m1.ID); err == nil {
		t.Error("용량 초과 시 가장 오래된 것부터 지워야 함(m1)")
	}
	if _, err := rs.Meta(m2.ID); err == nil {
		t.Error("용량 초과 시 오래된 것부터 지워야 함(m2)")
	}
	if _, err := rs.Meta(m3.ID); err != nil {
		t.Error("가장 새 것은 남아야 함")
	}
	// TTL
	rs.MaxBytes = 1 << 30
	rs.now = func() time.Time { return t0.Add(3 * time.Hour) }
	rs.Cleanup()
	if _, err := os.Stat(filepath.Join(rs.Dir, m3.ID+".csv")); !os.IsNotExist(err) {
		t.Error("TTL 지나면 파일을 지워야 함")
	}
	if _, err := os.Stat(filepath.Join(rs.Dir, "unrelated.txt")); err != nil {
		t.Error("결과 파일이 아닌 것은 건드리지 않음")
	}
}

func TestResourcesRead(t *testing.T) {
	f := newFakeSGIS(t)
	rs := newStore(t)
	s := pagedServer(f, rs, []Cred{{defKey, defSecret}})
	text, _, _ := call(t, s, "sgis_population", map[string]interface{}{"year": "2020", "adm_cd": "11", "low_search": "2"})
	id := headOf(t, text, "columns")["result_id"].(string)
	uri := "sgis://results/" + id + ".csv"
	res, _ := rpc(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":%q}}`, uri),
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"sgis://results/AAAAAAAAAAAAAAAAAAAAAAAA.csv"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`,
	)
	caps := res["1"]["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	if _, ok := caps["resources"]; !ok {
		t.Error("capabilities.resources 필요")
	}
	c := res["2"]["result"].(map[string]interface{})["contents"].([]interface{})[0].(map[string]interface{})
	file, _ := os.ReadFile(filepath.Join(rs.Dir, id+".csv"))
	if c["uri"] != uri || c["mimeType"] != "text/csv" || c["text"] != string(file) {
		t.Errorf("resources/read 내용: %v", c["mimeType"])
	}
	recs, _ := csv.NewReader(strings.NewReader(c["text"].(string))).ReadAll()
	if len(recs) != fakePopRows+1 || recs[1][1] != "서울특별시 테스트구 동000" {
		t.Errorf("CSV 전체 행: %d", len(recs))
	}
	if res["3"]["error"] == nil {
		t.Error("없는 결과는 오류")
	}
	if _, ok := res["4"]["result"]; !ok {
		t.Error("resources/list 결과")
	}
}

func TestHTTPResultsDownloadToken(t *testing.T) {
	f := newFakeSGIS(t)
	rs := newStore(t)
	s := pagedServer(f, rs, []Cred{{defKey, defSecret}})
	text, _, _ := call(t, s, "sgis_population", map[string]interface{}{"year": "2020", "adm_cd": "11", "low_search": "2"})
	id := headOf(t, text, "columns")["result_id"].(string)
	ts, _ := newHTTPTest(t, s, HTTPOptions{Token: "dl-token"})
	get := func(path, auth string) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, a := range []string{"", "Bearer nope"} {
		r := get("/results/"+id+".csv", a)
		r.Body.Close()
		if r.StatusCode != http.StatusUnauthorized {
			t.Errorf("토큰 없거나 틀리면 401: %d", r.StatusCode)
		}
	}
	r := get("/results/"+id+".csv", "Bearer dl-token")
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	file, _ := os.ReadFile(filepath.Join(rs.Dir, id+".csv"))
	if r.StatusCode != 200 || string(body) != string(file) || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/csv") ||
		r.Header.Get("Content-Length") != fmt.Sprint(len(file)) || !strings.Contains(r.Header.Get("Content-Disposition"), id+".csv") {
		t.Errorf("다운로드: %d %v", r.StatusCode, r.Header)
	}
	for _, p := range []string{"/results/" + id + ".geojson", "/results/" + id + ".jsonl", "/results/../x", "/results/nope.csv"} {
		r := get(p, "Bearer dl-token")
		r.Body.Close()
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("%s 는 404: %d", p, r.StatusCode)
		}
	}
	// 토큰 없는 loopback 서버는 그대로
	_, c2 := newHTTPTest(t, s, HTTPOptions{})
	r2, _ := http.Get(strings.TrimSuffix(c2.url, "/mcp") + "/results/" + id + ".csv")
	r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Errorf("토큰 없는 loopback 은 그대로 200: %d", r2.StatusCode)
	}
}

func TestBoundaryPagingGeoJSON(t *testing.T) {
	f := newFakeSGIS(t)
	rs := newStore(t)
	s := pagedServer(f, rs, []Cred{{defKey, defSecret}})
	text, isErr, content := call(t, s, "sgis_boundary", map[string]interface{}{
		"year": "2024", "adm_cd": "11", "low_search": "2", "decimals": 4, "properties": []string{"adm_cd"}})
	if isErr {
		t.Fatal(text)
	}
	var fc struct {
		Total    int               `json:"total_rows"`
		Next     string            `json:"next_cursor"`
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal([]byte(text), &fc); err != nil || fc.Type != "FeatureCollection" || fc.Next == "" || fc.Total != fakeGeoFeats {
		t.Fatalf("첫 쪽은 요약 칸 붙은 유효한 FeatureCollection: %v %.200s", err, text)
	}
	link := content[1].(map[string]interface{})
	if link["mimeType"] != "application/geo+json" || !strings.HasSuffix(link["uri"].(string), ".geojson") {
		t.Errorf("geojson resource_link: %v", link)
	}
	feats := fc.Features
	for cur := fc.Next; cur != ""; {
		ptext, isErr, _ := call(t, s, "sgis_result_page", map[string]interface{}{"cursor": cur})
		if isErr {
			t.Fatal(ptext)
		}
		var p struct {
			Next     string            `json:"next_cursor"`
			Type     string            `json:"type"`
			Features []json.RawMessage `json:"features"`
		}
		if err := json.Unmarshal([]byte(ptext), &p); err != nil || p.Type != "FeatureCollection" || len(ptext) > testPageBytes {
			t.Fatalf("쪽마다 유효한 GeoJSON: %v", err)
		}
		feats = append(feats, p.Features...)
		cur = p.Next
	}
	if len(feats) != fakeGeoFeats {
		t.Fatalf("feature 합계 %d, want %d", len(feats), fakeGeoFeats)
	}
	var f0 struct {
		Properties map[string]interface{} `json:"properties"`
		Geometry   struct {
			Coordinates [][][]float64 `json:"coordinates"`
		} `json:"geometry"`
	}
	_ = json.Unmarshal(feats[0], &f0)
	if len(f0.Properties) != 1 || f0.Properties["adm_cd"] != "11010500" {
		t.Errorf("properties 고르기: %v", f0.Properties)
	}
	x := f0.Geometry.Coordinates[0][0][0]
	if x < 126 || x > 128 || x != float64(int(x*10000+0.5))/10000 {
		t.Errorf("WGS84 재투영 + 소수 4자리: %v", x)
	}
	// 전체 파일
	var full struct {
		Features []json.RawMessage `json:"features"`
	}
	b, _ := os.ReadFile(filepath.Join(rs.Dir, strings.TrimPrefix(link["uri"].(string), "sgis://results/")))
	if err := json.Unmarshal(b, &full); err != nil || len(full.Features) != fakeGeoFeats {
		t.Errorf("저장된 .geojson 전체: %v %d", err, len(full.Features))
	}
}

func TestResultFilesHaveNoSecrets(t *testing.T) {
	f := newFakeSGIS(t)
	rs := newStore(t)
	s := pagedServer(f, rs, nil)
	meta := fmt.Sprintf(`{"kr.pes/keys":["%s:%s"]}`, reqKey, reqSecret)
	rpc(t, s,
		callLine(1, "sgis_population", `{"year":"2020","adm_cd":"11","low_search":"2","save":true}`, meta),
		callLine(2, "sgis_boundary", `{"year":"2024","adm_cd":"11"}`, meta),
	)
	entries, _ := os.ReadDir(rs.Dir)
	if len(entries) < 4 {
		t.Fatalf("결과 파일이 저장돼야 함: %d", len(entries))
	}
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(rs.Dir, e.Name()))
		for _, sec := range []string{reqKey, reqSecret, "TOKEN-" + reqKey + "-zzzz", "accessToken", "consumer"} {
			if strings.Contains(string(b), sec) || strings.Contains(e.Name(), sec) {
				t.Errorf("%s 에 비밀(%s…)이 들어감", e.Name(), sec[:4])
			}
		}
	}
}
