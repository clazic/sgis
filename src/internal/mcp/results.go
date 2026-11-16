package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clazic/sgis/internal/config"
)

// 결과 저장소: 큰 결과(행 목록 · GeoJSON feature 목록)를 디스크에 두고 쪽 나누기·내려받기에 씁니다.
// 파일: <id>.jsonl(한 줄 = 행 배열 또는 feature) · <id>.csv 또는 <id>.geojson · <id>.meta.json.
// 공개 통계 응답만 저장하며 키·시크릿·토큰은 절대 넣지 않습니다.

const (
	defaultResultsTTL      = time.Hour
	defaultResultsMaxBytes = int64(1) << 30
	resultsCleanupEvery    = 10 * time.Minute
	maxResourceReadBytes   = 20 << 20

	kindTable = "table"
	kindGeo   = "geojson"
)

// ErrResultExpired는 결과가 없거나 보관 시간이 지난 경우입니다.
var ErrResultExpired = errors.New("결과가 만료됐어요 — 다시 조회하세요")

var resultIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)

// ResultStore는 결과 파일 폴더입니다. 여러 프로세스가 같은 폴더를 써도 됩니다(임시 파일 → rename).
type ResultStore struct {
	Dir      string
	TTL      time.Duration
	MaxBytes int64
	now      func() time.Time
}

type resultMeta struct {
	ID          string                 `json:"id"`
	Tool        string                 `json:"tool"`
	Created     time.Time              `json:"created"`
	Rows        int                    `json:"rows"`
	Columns     []string               `json:"columns,omitempty"`
	Bytes       int64                  `json:"bytes"`
	Formats     []string               `json:"formats"`
	ArgsSummary map[string]interface{} `json:"args_summary,omitempty"`
	Kind        string                 `json:"kind"`
}

// ResultStoreFromEnv는 MCP_RESULTS_DIR · MCP_RESULTS_TTL · MCP_RESULTS_MAX_BYTES 로 저장소를 만듭니다.
// 폴더 기본값은 <sgis 설정 폴더>/cache/mcp-results (~/.sgis/cache/mcp-results).
func ResultStoreFromEnv() (*ResultStore, error) {
	rs := &ResultStore{Dir: os.Getenv("MCP_RESULTS_DIR"), TTL: defaultResultsTTL, MaxBytes: defaultResultsMaxBytes}
	if rs.Dir == "" {
		rs.Dir = filepath.Join(config.ConfigDir(), "cache", "mcp-results")
	}
	if v := strings.TrimSpace(os.Getenv("MCP_RESULTS_TTL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			secs, err2 := strconv.Atoi(v)
			if err2 != nil || secs <= 0 {
				return nil, fmt.Errorf("MCP_RESULTS_TTL 형식 오류(예: 1h, 3600)")
			}
			d = time.Duration(secs) * time.Second
		}
		rs.TTL = d
	}
	if v := strings.TrimSpace(os.Getenv("MCP_RESULTS_MAX_BYTES")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("MCP_RESULTS_MAX_BYTES 형식 오류(바이트 정수)")
		}
		rs.MaxBytes = n
	}
	return rs, nil
}

func (rs *ResultStore) clock() time.Time {
	if rs.now != nil {
		return rs.now()
	}
	return time.Now()
}

func newResultID() string {
	b := make([]byte, 18) // 144bit
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// writeAtomic은 같은 폴더의 임시 파일에 쓴 뒤 rename 합니다.
func (rs *ResultStore) writeAtomic(name string, fill func(w io.Writer) error) (int64, error) {
	if err := os.MkdirAll(rs.Dir, 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(rs.Dir, "."+name+".tmp-*")
	if err != nil {
		return 0, err
	}
	bw := bufio.NewWriterSize(tmp, 256<<10)
	err = fill(bw)
	if err == nil {
		err = bw.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		_ = os.Chmod(tmp.Name(), 0o644)
		err = os.Rename(tmp.Name(), filepath.Join(rs.Dir, name))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return 0, err
	}
	st, err := os.Stat(filepath.Join(rs.Dir, name))
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func (rs *ResultStore) writeMeta(m *resultMeta) error {
	_, err := rs.writeAtomic(m.ID+".meta.json", func(w io.Writer) error {
		return json.NewEncoder(w).Encode(m)
	})
	return err
}

// SaveTable은 표를 jsonl + csv 로 저장합니다(meta 는 마지막에 — meta 가 보이면 완성본).
func (rs *ResultStore) SaveTable(tool string, args map[string]interface{}, t *table) (*resultMeta, error) {
	id := newResultID()
	if _, err := rs.writeAtomic(id+".jsonl", func(w io.Writer) error {
		for _, r := range t.Rows {
			b, err := json.Marshal(r)
			if err != nil {
				return err
			}
			if _, err := w.Write(append(b, '\n')); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	size, err := rs.writeAtomic(id+".csv", func(w io.Writer) error { return writeCSV(w, t.Columns, t.Rows) })
	if err != nil {
		return nil, err
	}
	m := &resultMeta{ID: id, Tool: tool, Created: rs.clock(), Rows: len(t.Rows), Columns: t.Columns,
		Bytes: size, Formats: []string{"columns", "csv", "json"}, ArgsSummary: args, Kind: kindTable}
	return m, rs.writeMeta(m)
}

// SaveFeatures는 GeoJSON feature 목록을 jsonl + geojson 으로 저장합니다.
func (rs *ResultStore) SaveFeatures(tool string, args map[string]interface{}, feats []interface{}) (*resultMeta, error) {
	id := newResultID()
	lines := make([][]byte, len(feats))
	for i, f := range feats {
		b, err := json.Marshal(f)
		if err != nil {
			return nil, err
		}
		lines[i] = b
	}
	if _, err := rs.writeAtomic(id+".jsonl", func(w io.Writer) error {
		for _, l := range lines {
			if _, err := w.Write(append(l, '\n')); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	size, err := rs.writeAtomic(id+".geojson", func(w io.Writer) error {
		if _, err := io.WriteString(w, `{"type":"FeatureCollection","features":[`); err != nil {
			return err
		}
		for i, l := range lines {
			if i > 0 {
				if _, err := io.WriteString(w, ","); err != nil {
					return err
				}
			}
			if _, err := w.Write(l); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "]}\n")
		return err
	})
	if err != nil {
		return nil, err
	}
	m := &resultMeta{ID: id, Tool: tool, Created: rs.clock(), Rows: len(feats), Bytes: size,
		Formats: []string{"geojson"}, ArgsSummary: args, Kind: kindGeo}
	return m, rs.writeMeta(m)
}

// Meta는 결과 메타를 읽습니다. 없거나 만료면 ErrResultExpired.
func (rs *ResultStore) Meta(id string) (*resultMeta, error) {
	if !resultIDRe.MatchString(id) {
		return nil, ErrResultExpired
	}
	b, err := os.ReadFile(filepath.Join(rs.Dir, id+".meta.json"))
	if err != nil {
		return nil, ErrResultExpired
	}
	var m resultMeta
	if json.Unmarshal(b, &m) != nil || m.ID != id {
		return nil, ErrResultExpired
	}
	if rs.clock().Sub(m.Created) > rs.TTL {
		return nil, ErrResultExpired
	}
	return &m, nil
}

// ReadLines는 jsonl 의 [off, off+max) 줄을 읽습니다.
func (rs *ResultStore) ReadLines(id string, off, max int) ([]json.RawMessage, error) {
	f, err := os.Open(filepath.Join(rs.Dir, id+".jsonl"))
	if err != nil {
		return nil, ErrResultExpired
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var out []json.RawMessage
	for i := 0; sc.Scan(); i++ {
		if i < off {
			continue
		}
		if len(out) >= max {
			break
		}
		out = append(out, json.RawMessage(append([]byte(nil), sc.Bytes()...)))
	}
	return out, sc.Err()
}

// downloadExt는 결과 종류별 내려받기 파일 확장자입니다.
func downloadExt(kind string) string {
	if kind == kindGeo {
		return "geojson"
	}
	return "csv"
}

func mimeFor(ext string) string {
	if ext == "geojson" {
		return "application/geo+json"
	}
	return "text/csv; charset=utf-8"
}

// FilePath는 "<id>.<ext>" 내려받기 파일 경로를 돌려줍니다(만료·형식 확인).
func (rs *ResultStore) FilePath(name string) (string, *resultMeta, error) {
	dot := strings.Index(name, ".")
	if dot <= 0 {
		return "", nil, ErrResultExpired
	}
	id, ext := name[:dot], name[dot+1:]
	m, err := rs.Meta(id)
	if err != nil {
		return "", nil, err
	}
	if ext != downloadExt(m.Kind) {
		return "", nil, ErrResultExpired
	}
	p := filepath.Join(rs.Dir, id+"."+ext)
	if _, err := os.Stat(p); err != nil {
		return "", nil, ErrResultExpired
	}
	return p, m, nil
}

// Cleanup은 만료된 결과, 오래된 임시 파일, 상한을 넘는 오래된 결과를 지웁니다.
func (rs *ResultStore) Cleanup() {
	entries, err := os.ReadDir(rs.Dir)
	if err != nil {
		return
	}
	now := rs.clock()
	type group struct {
		files   []string
		size    int64
		created time.Time
		hasMeta bool
	}
	groups := map[string]*group{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		info, err := e.Info()
		if err != nil {
			continue
		}
		if strings.HasPrefix(name, ".") {
			if strings.Contains(name, ".tmp-") && now.Sub(info.ModTime()) > rs.TTL {
				_ = os.Remove(filepath.Join(rs.Dir, name))
			}
			continue
		}
		dot := strings.Index(name, ".")
		if dot <= 0 || !resultIDRe.MatchString(name[:dot]) {
			continue // 남의 파일은 건드리지 않음
		}
		id := name[:dot]
		g := groups[id]
		if g == nil {
			g = &group{created: info.ModTime()}
			groups[id] = g
		}
		g.files = append(g.files, name)
		g.size += info.Size()
		if strings.HasSuffix(name, ".meta.json") {
			g.hasMeta = true
			if b, err := os.ReadFile(filepath.Join(rs.Dir, name)); err == nil {
				var m resultMeta
				if json.Unmarshal(b, &m) == nil && !m.Created.IsZero() {
					g.created = m.Created
				}
			}
		}
	}
	remove := func(g *group) {
		for _, f := range g.files {
			_ = os.Remove(filepath.Join(rs.Dir, f))
		}
	}
	var live []*group
	var total int64
	for _, g := range groups {
		if now.Sub(g.created) > rs.TTL {
			remove(g)
			continue
		}
		live = append(live, g)
		total += g.size
	}
	if total <= rs.MaxBytes {
		return
	}
	sort.Slice(live, func(i, j int) bool { return live[i].created.Before(live[j].created) })
	for _, g := range live {
		if total <= rs.MaxBytes {
			break
		}
		remove(g)
		total -= g.size
	}
}

// RunCleanup은 시작할 때와 10분마다 Cleanup 을 돌립니다(ctx 끝나면 멈춤).
func (rs *ResultStore) RunCleanup(ctx context.Context) {
	rs.Cleanup()
	t := time.NewTicker(resultsCleanupEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rs.Cleanup()
		}
	}
}

// writeCSV는 머리줄 + 행을 씁니다.
func writeCSV(w io.Writer, cols []string, rows [][]interface{}) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	rec := make([]string, len(cols))
	for _, r := range rows {
		for i := range rec {
			rec[i] = ""
			if i < len(r) {
				rec[i] = cellString(r[i])
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func cellString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
