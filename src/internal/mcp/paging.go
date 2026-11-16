package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/clazic/sgis/internal/output"
)

const resourcePrefix = "sgis://results/"

// download는 저장된 결과의 내려받기 안내(요약 칸 download)입니다.
func (s *Server) download(m *resultMeta) []map[string]interface{} {
	ext := downloadExt(m.Kind)
	name := m.ID + "." + ext
	return []map[string]interface{}{{
		"uri": resourcePrefix + name, "name": name, "mimeType": linkMime(ext), "size": m.Bytes,
		"path": "/results/" + name, "rows": m.Rows,
	}}
}

func linkMime(ext string) string {
	if ext == "geojson" {
		return "application/geo+json"
	}
	return "text/csv"
}

// resourceLink는 tools/call content 에 붙는 resource_link 항목입니다(MCP 2025-06-18).
func resourceLink(m *resultMeta) map[string]interface{} {
	ext := downloadExt(m.Kind)
	name := m.ID + "." + ext
	return map[string]interface{}{
		"type": "resource_link", "uri": resourcePrefix + name, "name": name,
		"mimeType": linkMime(ext), "size": m.Bytes, "description": fmt.Sprintf("전체 %d행", m.Rows),
	}
}

func argsSummary(a map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range a {
		out[k] = v // 도구 인자에는 키가 없음(키는 _meta·머리글·환경변수로만 옴)
	}
	return out
}

// tableResult는 통계 결과(객체 배열)를 형식·거르기·집계·쪽 나누기·저장을 거쳐 텍스트로 만듭니다.
// 인자가 없고 작으면 지금처럼 원본 JSON 을 그대로 돌려줍니다(호환).
func (s *Server) tableResult(cc *callCtx, tool string, a map[string]interface{}, raw json.RawMessage) (string, error) {
	fmtArg, _ := a["format"].(string)
	filter, _ := a["filter"].(map[string]interface{})
	aggRaw := a["aggregate"]
	save, _ := a["save"].(bool)
	compact := compactJSON(raw)
	if fmtArg == "" && filter == nil && aggRaw == nil && !save && len(compact) <= s.pageBytes {
		return compact, nil
	}
	t, ok := decodeTable(raw)
	if !ok {
		if fmtArg == "" && filter == nil && aggRaw == nil {
			return capText(compact, s.pageBytes), nil
		}
		return "", errors.New("이 결과는 표(객체 배열)가 아니라 format/filter/aggregate 를 쓸 수 없어요")
	}
	return s.shapeTable(cc, tool, a, t)
}

func (s *Server) shapeTable(cc *callCtx, tool string, a map[string]interface{}, t *table) (string, error) {
	fmtArg, _ := a["format"].(string)
	filter, _ := a["filter"].(map[string]interface{})
	save, _ := a["save"].(bool)
	var extra []kv
	var err error
	if t, err = applyFilter(t, filter); err != nil {
		return "", err
	}
	if aggRaw, ok := a["aggregate"]; ok && aggRaw != nil {
		spec, err := parseAgg(aggRaw)
		if err != nil {
			return "", err
		}
		var skipped int
		if t, skipped, err = applyAggregate(t, spec); err != nil {
			return "", err
		}
		if skipped > 0 {
			extra = append(extra, kv{"skipped_non_numeric", skipped})
		}
	}
	format := fmtArg
	if format == "" {
		format = "json"
	}
	info := pageInfo{Total: len(t.Rows), Format: format, Extra: extra}
	if !save {
		if text := renderTable(t.Columns, t.Rows, info); len(text) <= s.pageBytes {
			return text, nil
		}
		if fmtArg == "" {
			info.Format = "columns"
			if text := renderTable(t.Columns, t.Rows, info); len(text) <= s.pageBytes {
				return text, nil
			}
		}
	}
	m, err := s.results.SaveTable(tool, argsSummary(a), t)
	if err != nil {
		s.logger.Printf("결과 저장 실패: %v", err)
		info.Extra = append(info.Extra, kv{"note", "결과를 저장하지 못해 첫 쪽만 담았어요 — 범위를 좁히세요"})
		text, _ := fitPage(len(t.Rows), func(i int) int { return rowSize(t.Rows[i]) },
			func(k int) string { return renderTable(t.Columns, t.Rows[:k], info) }, s.pageBytes)
		return text, nil
	}
	info.ResultID = m.ID
	info.Download = s.download(m)
	cc.addLink(resourceLink(m))
	return s.tablePage(t.Columns, t.Rows, 0, m.Rows, info), nil
}

// tablePage는 rows(=off 부터의 행들)에서 상한에 맞는 만큼 한 쪽을 만듭니다.
func (s *Server) tablePage(cols []string, rows [][]interface{}, off, total int, info pageInfo) string {
	text, _ := fitPage(len(rows), func(i int) int { return rowSize(rows[i]) }, func(k int) string {
		inf := info
		inf.NextOffset = 0
		if off+k < total {
			inf.NextOffset = off + k
		}
		return renderTable(cols, rows[:k], inf)
	}, s.pageBytes)
	return text
}

func (s *Server) geoPage(feats []json.RawMessage, off, total int, info pageInfo) string {
	text, _ := fitPage(len(feats), func(i int) int { return len(feats[i]) }, func(k int) string {
		inf := info
		inf.NextOffset = 0
		if off+k < total {
			inf.NextOffset = off + k
		}
		return renderGeo(feats[:k], inf)
	}, s.pageBytes)
	return text
}

// geoResult는 경계 GeoJSON 을 재투영·반올림·속성 고르기 후, 작으면 그대로, 크면 저장하고 feature 단위 첫 쪽을 줍니다.
func (s *Server) geoResult(cc *callCtx, tool string, a map[string]interface{}, raw json.RawMessage) (string, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("GeoJSON 파싱 실패: %v", err)
	}
	obj, _ := v.(map[string]interface{})
	if obj != nil {
		if _, hasErr := obj["errCd"]; hasErr && obj["type"] == nil {
			if cd, _ := obj["errCd"].(float64); cd != 0 {
				return "", fmt.Errorf("SGIS API 오류 (errCd=%v): %v", obj["errCd"], obj["errMsg"])
			}
		}
	}
	wgs84, _ := a["wgs84"].(bool)
	decimals := 1
	if wgs84 {
		v = output.ReprojectWGS84(v)
		decimals = 6
	}
	if d, ok := a["decimals"].(float64); ok {
		decimals = int(d)
	}
	v = roundCoords(v, decimals)
	obj, _ = v.(map[string]interface{})
	feats, _ := obj["features"].([]interface{})
	if props, ok := a["properties"].([]string); ok && feats != nil {
		keep := map[string]bool{}
		for _, p := range props {
			keep[p] = true
		}
		for _, f := range feats {
			if fm, ok := f.(map[string]interface{}); ok {
				if pm, ok := fm["properties"].(map[string]interface{}); ok {
					for k := range pm {
						if !keep[k] {
							delete(pm, k)
						}
					}
				}
			}
		}
	}
	save, _ := a["save"].(bool)
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if !save && len(b) <= s.pageBytes {
		return string(b), nil
	}
	if obj == nil || feats == nil {
		return capText(string(b), s.pageBytes), nil
	}
	m, err := s.results.SaveFeatures(tool, argsSummary(a), feats)
	raws := make([]json.RawMessage, len(feats))
	for i, f := range feats {
		raws[i], _ = json.Marshal(f)
	}
	info := pageInfo{Total: len(feats), Format: "geojson"}
	if err != nil {
		s.logger.Printf("결과 저장 실패: %v", err)
		info.Extra = []kv{{"note", "결과를 저장하지 못해 첫 쪽만 담았어요 — 범위를 좁히세요"}}
		text, _ := fitPage(len(raws), func(i int) int { return len(raws[i]) },
			func(k int) string { return renderGeo(raws[:k], info) }, s.pageBytes)
		return text, nil
	}
	info.ResultID = m.ID
	info.Download = s.download(m)
	cc.addLink(resourceLink(m))
	return s.geoPage(raws, 0, m.Rows, info), nil
}

// resultTools는 저장된 결과를 읽는 도구(키 불필요)입니다.
func resultTools() []*Tool {
	return []*Tool{
		{
			Name: "sgis_result_page", Title: "결과 다음 쪽", NoKeys: true,
			Description: "큰 결과의 next_cursor 로 다음 쪽을 받습니다(같은 형식, 다시 조회하지 않음, API 키 불필요). 마지막 쪽이면 next_cursor 가 없습니다. 결과는 1시간 보관.",
			Fields: []field{
				{Name: "cursor", Type: "string", Required: true, MaxLen: 512, Pattern: `^[A-Za-z0-9_-]+$`, Desc: "앞 응답의 next_cursor"},
				{Name: "limit", Type: "integer", Min: fptr(1), Max: fptr(10000), Desc: "이 쪽 최대 행 수(생략 시 약 200KB 까지)"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				c, err := decodeCursor(a["cursor"].(string))
				if err != nil {
					return "", err
				}
				m, err := s.results.Meta(c.ID)
				if err != nil {
					return "", err
				}
				if c.Offset >= m.Rows {
					return "", errors.New("cursor 가 결과 끝을 넘었어요")
				}
				if (m.Kind == kindGeo) != (c.Format == "geojson") {
					return "", errors.New("cursor 형식이 잘못됐어요")
				}
				max := 20000
				if l, ok := a["limit"].(float64); ok {
					max = int(l)
				}
				lines, err := s.results.ReadLines(m.ID, c.Offset, max)
				if err != nil {
					return "", err
				}
				info := pageInfo{Total: m.Rows, Format: c.Format, ResultID: m.ID, Download: s.download(m)}
				if m.Kind == kindGeo {
					return s.geoPage(lines, c.Offset, m.Rows, info), nil
				}
				rows, err := decodeRows(lines)
				if err != nil {
					return "", fmt.Errorf("저장된 결과를 읽지 못했어요: %v", err)
				}
				return s.tablePage(m.Columns, rows, c.Offset, m.Rows, info), nil
			},
		},
		{
			Name: "sgis_result_query", Title: "저장 결과 거르기·집계", NoKeys: true,
			Description: "저장된 표 결과(result_id)에 filter/aggregate 를 걸어 다시 받습니다(SGIS 를 다시 부르지 않음, API 키 불필요). 결과가 크면 다시 저장해 새 result_id·next_cursor 를 줍니다.",
			Fields: append([]field{
				{Name: "result_id", Type: "string", Required: true, Pattern: `^[A-Za-z0-9_-]{22,64}$`, MaxLen: 64, Desc: "앞 응답의 result_id"},
			}, tableFields()...),
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				m, err := s.results.Meta(a["result_id"].(string))
				if err != nil {
					return "", err
				}
				if m.Kind != kindTable {
					return "", errors.New("표 결과에만 쓸 수 있어요(GeoJSON 은 sgis_result_page)")
				}
				lines, err := s.results.ReadLines(m.ID, 0, m.Rows)
				if err != nil {
					return "", err
				}
				rows, err := decodeRows(lines)
				if err != nil {
					return "", fmt.Errorf("저장된 결과를 읽지 못했어요: %v", err)
				}
				return s.shapeTable(cc, "sgis_result_query", a, &table{Columns: m.Columns, Rows: rows})
			},
		},
	}
}

// readResource는 resources/read 를 처리합니다.
func (s *Server) readResource(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(params, &p) != nil || !strings.HasPrefix(p.URI, resourcePrefix) {
		return nil, &rpcError{Code: -32602, Message: "uri 는 " + resourcePrefix + "<id>.<csv|geojson> 이어야 해요"}
	}
	path, m, err := s.results.FilePath(strings.TrimPrefix(p.URI, resourcePrefix))
	if err != nil {
		return nil, &rpcError{Code: -32002, Message: ErrResultExpired.Error()}
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, &rpcError{Code: -32002, Message: ErrResultExpired.Error()}
	}
	if st.Size() > maxResourceReadBytes {
		return nil, &rpcError{Code: -32603, Message: fmt.Sprintf("파일이 %dMB 라 20MB 를 넘어요 — 내려받기(HTTP GET /results/…)를 쓰세요", st.Size()>>20)}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, &rpcError{Code: -32002, Message: ErrResultExpired.Error()}
	}
	return map[string]interface{}{"contents": []map[string]interface{}{{
		"uri": p.URI, "mimeType": linkMime(downloadExt(m.Kind)), "text": string(b),
	}}}, nil
}
