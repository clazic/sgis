package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// table은 SGIS 통계 결과(객체 배열)를 열 순서를 지켜 담은 표입니다.
type table struct {
	Columns []string
	Rows    [][]interface{}
}

// decodeTable은 객체 배열 JSON 을 열 순서(처음 나온 순서)를 지켜 표로 읽습니다. 값은 json.Number 를 씁니다.
func decodeTable(raw json.RawMessage) (*table, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('[') {
		return nil, false
	}
	t := &table{}
	idx := map[string]int{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil || tok != json.Delim('{') {
			return nil, false
		}
		row := make([]interface{}, len(t.Columns))
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, false
			}
			k, _ := kt.(string)
			var v interface{}
			if err := dec.Decode(&v); err != nil {
				return nil, false
			}
			i, ok := idx[k]
			if !ok {
				i = len(t.Columns)
				idx[k] = i
				t.Columns = append(t.Columns, k)
			}
			for len(row) <= i {
				row = append(row, nil)
			}
			row[i] = v
		}
		if _, err := dec.Token(); err != nil { // '}'
			return nil, false
		}
		t.Rows = append(t.Rows, row)
	}
	for i := range t.Rows {
		for len(t.Rows[i]) < len(t.Columns) {
			t.Rows[i] = append(t.Rows[i], nil)
		}
	}
	return t, true
}

func decodeRows(lines []json.RawMessage) ([][]interface{}, error) {
	out := make([][]interface{}, 0, len(lines))
	for _, l := range lines {
		dec := json.NewDecoder(bytes.NewReader(l))
		dec.UseNumber()
		var r []interface{}
		if err := dec.Decode(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (t *table) col(name string) int {
	for i, c := range t.Columns {
		if c == name {
			return i
		}
	}
	return -1
}

// ── filter ─────────────────────────────────────────────────────────────────

// applyFilter: {"col": ["a","b"]} 목록 안, {"col": {"from":x,"to":y}} 범위(둘 다 숫자면 숫자 비교), {"col": "a"} 같음.
func applyFilter(t *table, f map[string]interface{}) (*table, error) {
	if len(f) == 0 {
		return t, nil
	}
	type cond struct {
		i        int
		set      map[string]bool
		from, to *string
	}
	var conds []cond
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		i := t.col(k)
		if i < 0 {
			return nil, fmt.Errorf("filter: 없는 열 %q (열: %s)", truncateRunes(k, 40), strings.Join(t.Columns, ", "))
		}
		c := cond{i: i}
		switch v := f[k].(type) {
		case []interface{}:
			c.set = map[string]bool{}
			for _, x := range v {
				c.set[cellString(normNum(x))] = true
			}
		case map[string]interface{}:
			for kk := range v {
				if kk != "from" && kk != "to" {
					return nil, fmt.Errorf("filter: 범위는 from/to 만 써요")
				}
			}
			if x, ok := v["from"]; ok && x != nil {
				s := cellString(normNum(x))
				c.from = &s
			}
			if x, ok := v["to"]; ok && x != nil {
				s := cellString(normNum(x))
				c.to = &s
			}
		case string, float64, bool:
			c.set = map[string]bool{cellString(normNum(v)): true}
		default:
			return nil, fmt.Errorf("filter: %q 값은 목록·{from,to}·값 중 하나", truncateRunes(k, 40))
		}
		conds = append(conds, c)
	}
	out := &table{Columns: t.Columns}
	for _, r := range t.Rows {
		ok := true
		for _, c := range conds {
			s := cellString(r[c.i])
			if c.set != nil && !c.set[s] {
				ok = false
			}
			if c.from != nil && compareCell(s, *c.from) < 0 {
				ok = false
			}
			if c.to != nil && compareCell(s, *c.to) > 0 {
				ok = false
			}
			if !ok {
				break
			}
		}
		if ok {
			out.Rows = append(out.Rows, r)
		}
	}
	return out, nil
}

// normNum은 JSON 숫자(float64)를 정수면 정수 문자열로 바꿉니다.
func normNum(v interface{}) interface{} {
	if f, ok := v.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return v
}

func parseNum(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func compareCell(a, b string) int {
	if x, ok := parseNum(a); ok {
		if y, ok2 := parseNum(b); ok2 {
			switch {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		}
	}
	return strings.Compare(a, b)
}

// ── aggregate ──────────────────────────────────────────────────────────────

type aggSpec struct {
	GroupBy []string
	Op      string
	Value   string
}

func parseAgg(v interface{}) (*aggSpec, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, errors.New("aggregate 는 {group_by, op, value} 객체예요")
	}
	a := &aggSpec{}
	for k := range m {
		if k != "group_by" && k != "op" && k != "value" {
			return nil, fmt.Errorf("aggregate: 모르는 칸 %q", truncateRunes(k, 30))
		}
	}
	if gb, ok := m["group_by"].([]interface{}); ok {
		for _, g := range gb {
			s, ok := g.(string)
			if !ok {
				return nil, errors.New("aggregate.group_by 는 열 이름 목록이에요")
			}
			a.GroupBy = append(a.GroupBy, s)
		}
	} else if m["group_by"] != nil {
		return nil, errors.New("aggregate.group_by 는 열 이름 목록이에요")
	}
	a.Op, _ = m["op"].(string)
	switch a.Op {
	case "sum", "avg", "min", "max", "count":
	default:
		return nil, errors.New("aggregate.op 는 sum|avg|min|max|count 중 하나예요")
	}
	a.Value, _ = m["value"].(string)
	if a.Value == "" && a.Op != "count" {
		return nil, errors.New("aggregate.value(숫자 열 이름)가 필요해요")
	}
	return a, nil
}

// applyAggregate는 group_by 로 묶어 op 를 계산합니다. 숫자가 아닌 값은 건너뛰고 그 수를 돌려줍니다.
func applyAggregate(t *table, a *aggSpec) (*table, int, error) {
	gi := make([]int, len(a.GroupBy))
	for i, g := range a.GroupBy {
		if gi[i] = t.col(g); gi[i] < 0 {
			return nil, 0, fmt.Errorf("aggregate: 없는 열 %q", truncateRunes(g, 40))
		}
	}
	vi := -1
	if a.Value != "" {
		if vi = t.col(a.Value); vi < 0 {
			return nil, 0, fmt.Errorf("aggregate: 없는 열 %q", truncateRunes(a.Value, 40))
		}
	}
	type acc struct {
		key      []interface{}
		n        int
		sum      float64
		min, max float64
	}
	var order []string
	groups := map[string]*acc{}
	skipped := 0
	for _, r := range t.Rows {
		kparts := make([]string, len(gi))
		key := make([]interface{}, len(gi))
		for i, c := range gi {
			key[i] = r[c]
			kparts[i] = cellString(r[c])
		}
		ks := strings.Join(kparts, "\x00")
		g := groups[ks]
		if g == nil {
			g = &acc{key: key, min: math.Inf(1), max: math.Inf(-1)}
			groups[ks] = g
			order = append(order, ks)
		}
		if a.Op == "count" && vi < 0 {
			g.n++
			continue
		}
		x, ok := parseNum(cellString(r[vi]))
		if !ok {
			skipped++
			continue
		}
		g.n++
		g.sum += x
		g.min = math.Min(g.min, x)
		g.max = math.Max(g.max, x)
	}
	name := a.Op
	if a.Value != "" {
		name = a.Op + "_" + a.Value
	}
	out := &table{Columns: append(append([]string{}, a.GroupBy...), name, "n")}
	for _, ks := range order {
		g := groups[ks]
		var v interface{}
		switch a.Op {
		case "sum":
			v = g.sum
		case "avg":
			if g.n > 0 {
				v = g.sum / float64(g.n)
			}
		case "min":
			if g.n > 0 {
				v = g.min
			}
		case "max":
			if g.n > 0 {
				v = g.max
			}
		case "count":
			v = g.n
		}
		row := append(append([]interface{}{}, g.key...), v, g.n)
		out.Rows = append(out.Rows, row)
	}
	return out, skipped, nil
}

// ── 쪽 렌더링 ──────────────────────────────────────────────────────────────

type kv struct {
	K string
	V interface{}
}

// orderedJSON은 칸 순서를 지켜 JSON 객체를 만듭니다(값이 json.RawMessage 면 그대로).
func orderedJSON(fields []kv) []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.K)
		b.Write(k)
		b.WriteByte(':')
		v, err := json.Marshal(f.V)
		if err != nil {
			v = []byte("null")
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// pageInfo는 응답 맨 앞 요약 칸입니다.
type pageInfo struct {
	Total      int
	Format     string
	ResultID   string
	NextOffset int // 0 이면 다음 쪽 없음
	Download   []map[string]interface{}
	Extra      []kv
}

func (p pageInfo) fields(returned int) []kv {
	f := []kv{{"total_rows", p.Total}, {"returned_rows", returned}, {"truncated", p.NextOffset > 0}, {"format", p.Format}}
	if p.ResultID != "" {
		f = append(f, kv{"result_id", p.ResultID})
	}
	if p.NextOffset > 0 {
		f = append(f, kv{"next_cursor", encodeCursor(p.ResultID, p.NextOffset, p.Format)})
	}
	if len(p.Download) > 0 {
		f = append(f, kv{"download", p.Download})
	}
	return append(f, p.Extra...)
}

// 이름 열 → 코드 열 (columns 형식에서 dict 로 한 번만 보냄)
var dictPairs = [][2]string{{"adm_cd", "adm_nm"}, {"cd", "addr_name"}}

func rowObject(cols []string, r []interface{}) json.RawMessage {
	f := make([]kv, len(cols))
	for i, c := range cols {
		var v interface{}
		if i < len(r) {
			v = r[i]
		}
		f[i] = kv{c, v}
	}
	return orderedJSON(f)
}

// renderTable은 한 쪽의 표를 형식대로 텍스트로 만듭니다.
func renderTable(cols []string, rows [][]interface{}, info pageInfo) string {
	head := info.fields(len(rows))
	switch info.Format {
	case "csv":
		var b bytes.Buffer
		b.Write(orderedJSON(head))
		b.WriteByte('\n')
		_ = writeCSV(&b, cols, rows)
		return b.String()
	case "json":
		objs := make([]json.RawMessage, len(rows))
		for i, r := range rows {
			objs[i] = rowObject(cols, r)
		}
		return string(orderedJSON(append(head, kv{"data", objs})))
	default: // columns
		outCols := cols
		outRows := rows
		dict := map[string]map[string]string{}
		dictCols := map[string]string{}
		drop := map[int]bool{}
		for _, p := range dictPairs {
			ci, ni := indexOf(cols, p[0]), indexOf(cols, p[1])
			if ci < 0 || ni < 0 {
				continue
			}
			m := map[string]string{}
			ok := true
			for _, r := range rows {
				code, name := cellString(r[ci]), cellString(r[ni])
				if prev, seen := m[code]; seen && prev != name {
					ok = false
					break
				}
				m[code] = name
			}
			if ok && len(rows) > 0 {
				dict[p[0]] = m
				dictCols[p[0]] = p[1]
				drop[ni] = true
			}
		}
		if len(drop) > 0 {
			outCols = nil
			for i, c := range cols {
				if !drop[i] {
					outCols = append(outCols, c)
				}
			}
			outRows = make([][]interface{}, len(rows))
			for j, r := range rows {
				nr := make([]interface{}, 0, len(outCols))
				for i, v := range r {
					if !drop[i] {
						nr = append(nr, v)
					}
				}
				outRows[j] = nr
			}
		}
		f := append(head, kv{"columns", outCols}, kv{"rows", outRows})
		if len(dict) > 0 {
			f = append(f, kv{"dict", dict}, kv{"dict_columns", dictCols})
		}
		return string(orderedJSON(f))
	}
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// renderGeo는 feature 들을 요약 칸이 앞에 붙은 유효한 FeatureCollection 으로 만듭니다.
func renderGeo(feats []json.RawMessage, info pageInfo) string {
	f := append(info.fields(len(feats)), kv{"type", "FeatureCollection"}, kv{"features", feats})
	return string(orderedJSON(f))
}

// fitPage는 render(k) 가 limit 이하가 되는 가장 큰 k(앞에서부터 k개)를 찾습니다. 최소 1개.
func fitPage(n int, sizes func(i int) int, render func(k int) string, limit int) (string, int) {
	if n == 0 {
		return render(0), 0
	}
	budget := limit * 9 / 10
	k, acc := 0, 512
	for k < n {
		acc += sizes(k) + 1
		if acc > budget && k > 0 {
			break
		}
		k++
	}
	for {
		text := render(k)
		if len(text) <= limit || k <= 1 {
			return text, k
		}
		nk := int(float64(k) * float64(limit) / float64(len(text)) * 0.95)
		if nk >= k {
			nk = k - 1
		}
		if nk < 1 {
			nk = 1
		}
		k = nk
	}
}

func rowSize(r []interface{}) int {
	b, _ := json.Marshal(r)
	return len(b)
}

// ── cursor ─────────────────────────────────────────────────────────────────

type cursor struct {
	ID     string `json:"i"`
	Offset int    `json:"o"`
	Format string `json:"f"`
}

func encodeCursor(id string, off int, format string) string {
	b, _ := json.Marshal(cursor{id, off, format})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.New("cursor 형식이 잘못됐어요")
	}
	var c cursor
	if json.Unmarshal(b, &c) != nil || c.ID == "" || c.Offset < 0 {
		return nil, errors.New("cursor 형식이 잘못됐어요")
	}
	switch c.Format {
	case "columns", "csv", "json", "geojson":
	default:
		return nil, errors.New("cursor 형식이 잘못됐어요")
	}
	return &c, nil
}
