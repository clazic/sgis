// Package output provides data formatting and output functionality for SGIS CLI.
// It supports rendering raw API result payloads to table, json, csv, geojson, and xlsx formats.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options configures how a result is rendered.
type Options struct {
	Format  string   // "table" | "json" | "csv" | "geojson" | "xlsx"
	OutFile string   // "" = stdout; otherwise write to this file path
	Title   string   // optional, used as xlsx sheet name
	Fields  []string // optional column projection (empty = all)
}

// FormatList returns the list of valid format names.
func FormatList() []string {
	return []string{"table", "json", "csv", "geojson", "xlsx"}
}

// Render formats an SGIS API result payload and writes it out.
// result is the raw `result` value from the API envelope (object or array).
func Render(result json.RawMessage, opts Options) error {
	// Normalise format
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = "table"
	}

	// xlsx requires OutFile
	if format == "xlsx" && opts.OutFile == "" {
		return fmt.Errorf("xlsx 포맷은 출력 파일 경로(--output)가 필요합니다")
	}

	// Validate format
	valid := false
	for _, f := range FormatList() {
		if format == f {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("알 수 없는 출력 형식: %s (사용 가능: %s)", format, strings.Join(FormatList(), ", "))
	}

	// Handle null / empty result
	if len(result) == 0 || string(result) == "null" {
		if opts.OutFile == "" {
			fmt.Println("(결과 없음)")
		}
		return nil
	}

	// Detect whether result is array or object
	trimmed := bytes.TrimSpace(result)
	isArray := len(trimmed) > 0 && trimmed[0] == '['

	switch format {
	case "json":
		return renderJSON(result, opts)
	case "csv":
		return renderCSV(result, isArray, opts)
	case "table":
		return renderTable(result, isArray, opts)
	case "geojson":
		return renderGeoJSON(result, opts)
	case "xlsx":
		return renderXLSX(result, isArray, opts)
	}
	return nil
}

// --- helpers ---

// parseArray unmarshals result into []map[string]interface{}.
func parseArray(result json.RawMessage) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}
	if err := json.Unmarshal(result, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// parseObject unmarshals result into map[string]interface{}.
func parseObject(result json.RawMessage) (map[string]interface{}, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(result, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// projectFields filters a row's keys to those in fields (preserving order).
// If fields is empty all keys are returned in sorted order.
func projectFields(row map[string]interface{}, fields []string) []string {
	if len(fields) > 0 {
		// return only requested fields that actually exist
		out := make([]string, 0, len(fields))
		for _, f := range fields {
			if _, ok := row[f]; ok {
				out = append(out, f)
			}
		}
		return out
	}
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// openOutput opens the appropriate writer. Caller must close the returned closer
// (which may be os.Stdout — that's fine, closing os.Stdout is a no-op for the
// purposes of this tool, but we return a nop-closer for stdout to be safe).
func openOutput(outFile string) (*os.File, func(), error) {
	if outFile == "" {
		return os.Stdout, func() {}, nil
	}
	// Ensure parent directory exists
	if dir := filepath.Dir(outFile); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("출력 디렉토리 생성 실패: %w", err)
		}
	}
	f, err := os.Create(outFile)
	if err != nil {
		return nil, nil, fmt.Errorf("파일 생성 실패 (%s): %w", outFile, err)
	}
	return f, func() { f.Close() }, nil
}

// isFlat returns true when all values in a map are scalar (not map/slice).
func isFlat(row map[string]interface{}) bool {
	for _, v := range row {
		switch v.(type) {
		case map[string]interface{}, []interface{}:
			return false
		}
	}
	return true
}

// formatValue converts an interface{} value to a display string.
func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
