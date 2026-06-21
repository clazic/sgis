package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// helpers

func rawJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// --- FormatList ---

func TestFormatList(t *testing.T) {
	formats := FormatList()
	want := map[string]bool{"table": true, "json": true, "csv": true, "geojson": true, "xlsx": true}
	if len(formats) != len(want) {
		t.Fatalf("FormatList len: got %d, want %d", len(formats), len(want))
	}
	for _, f := range formats {
		if !want[f] {
			t.Errorf("unexpected format %q in FormatList", f)
		}
	}
}

// --- JSON ---

func TestRenderJSON_Array(t *testing.T) {
	data := []map[string]interface{}{
		{"adm_cd": "11", "adm_nm": "서울특별시"},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.json")

	if err := Render(rawJSON(data), Options{Format: "json", OutFile: out}); err != nil {
		t.Fatalf("Render json: %v", err)
	}

	content, _ := os.ReadFile(out)
	// Should be pretty-printed
	if !strings.Contains(string(content), "\n") {
		t.Error("expected indented JSON output")
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed[0]["adm_cd"] != "11" {
		t.Errorf("unexpected value: %v", parsed[0])
	}
}

func TestRenderJSON_Object(t *testing.T) {
	data := map[string]interface{}{"key": "value", "num": 42}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.json")

	if err := Render(rawJSON(data), Options{Format: "json", OutFile: out}); err != nil {
		t.Fatalf("Render json object: %v", err)
	}
	content, _ := os.ReadFile(out)
	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

// --- Table (array) ---

func TestRenderTable_Array(t *testing.T) {
	data := []map[string]interface{}{
		{"name": "Seoul", "code": "11"},
		{"name": "Busan", "code": "26"},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.txt")

	if err := Render(rawJSON(data), Options{Format: "table", OutFile: out}); err != nil {
		t.Fatalf("Render table array: %v", err)
	}
	content, _ := os.ReadFile(out)
	s := string(content)
	if !strings.Contains(s, "Seoul") || !strings.Contains(s, "Busan") {
		t.Errorf("expected row data in table output, got:\n%s", s)
	}
	if !strings.Contains(s, "name") || !strings.Contains(s, "code") {
		t.Errorf("expected column headers in table output, got:\n%s", s)
	}
}

func TestRenderTable_Array_FieldProjection(t *testing.T) {
	data := []map[string]interface{}{
		{"name": "Seoul", "code": "11", "hidden": "secret"},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.txt")

	if err := Render(rawJSON(data), Options{Format: "table", OutFile: out, Fields: []string{"name"}}); err != nil {
		t.Fatalf("Render table with fields: %v", err)
	}
	content, _ := os.ReadFile(out)
	s := string(content)
	if strings.Contains(s, "hidden") || strings.Contains(s, "secret") {
		t.Errorf("projected-out field appeared in output:\n%s", s)
	}
	if !strings.Contains(s, "Seoul") {
		t.Errorf("expected 'Seoul' in output, got:\n%s", s)
	}
}

// --- Table (single object) ---

func TestRenderTable_Object(t *testing.T) {
	data := map[string]interface{}{"city": "Seoul", "population": 9700000}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.txt")

	if err := Render(rawJSON(data), Options{Format: "table", OutFile: out}); err != nil {
		t.Fatalf("Render table object: %v", err)
	}
	content, _ := os.ReadFile(out)
	s := string(content)
	if !strings.Contains(s, "city") || !strings.Contains(s, "Seoul") {
		t.Errorf("expected key/value in output, got:\n%s", s)
	}
}

// --- Table (nested → compact JSON fallback) ---

func TestRenderTable_Nested_Fallback(t *testing.T) {
	data := map[string]interface{}{
		"nested": map[string]interface{}{"a": 1},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.txt")

	if err := Render(rawJSON(data), Options{Format: "table", OutFile: out}); err != nil {
		t.Fatalf("Render table nested: %v", err)
	}
	content, _ := os.ReadFile(out)
	s := string(content)
	if !strings.Contains(s, "NOTE") {
		t.Errorf("expected fallback NOTE in output, got:\n%s", s)
	}
}

// --- CSV (array) ---

func TestRenderCSV_Array(t *testing.T) {
	data := []map[string]interface{}{
		{"adm_cd": "11", "adm_nm": "서울"},
		{"adm_cd": "26", "adm_nm": "부산"},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.csv")

	if err := Render(rawJSON(data), Options{Format: "csv", OutFile: out}); err != nil {
		t.Fatalf("Render csv array: %v", err)
	}
	content, _ := os.ReadFile(out)
	// Split on either \r\n or \n
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	// Remove trailing empty line
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	// header + 2 data rows
	if len(lines) < 3 {
		t.Fatalf("expected >=3 lines in CSV, got %d:\n%s", len(lines), string(content))
	}
	if !strings.Contains(lines[0], "adm_cd") {
		t.Errorf("header row missing: %s", lines[0])
	}
}

// --- CSV (single object) ---

func TestRenderCSV_Object(t *testing.T) {
	data := map[string]interface{}{"x": "1", "y": "2"}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.csv")

	if err := Render(rawJSON(data), Options{Format: "csv", OutFile: out}); err != nil {
		t.Fatalf("Render csv object: %v", err)
	}
	content, _ := os.ReadFile(out)
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if lines[0] != "key,value" {
		t.Errorf("expected 'key,value' header, got: %s", lines[0])
	}
	if len(lines) < 3 {
		t.Errorf("expected 3 lines (header + 2 keys), got %d", len(lines))
	}
}

// --- GeoJSON passthrough ---

func TestRenderGeoJSON_Passthrough(t *testing.T) {
	fc := map[string]interface{}{
		"type": "FeatureCollection",
		"features": []interface{}{
			map[string]interface{}{
				"type": "Feature",
				"geometry": map[string]interface{}{
					"type":        "Point",
					"coordinates": []interface{}{14140000.0, 4530000.0},
				},
				"properties": map[string]interface{}{"name": "test"},
			},
		},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.geojson")

	if err := Render(rawJSON(fc), Options{Format: "geojson", OutFile: out}); err != nil {
		t.Fatalf("Render geojson: %v", err)
	}
	content, _ := os.ReadFile(out)

	// Must be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("GeoJSON output is not valid JSON: %v", err)
	}
	if parsed["type"] != "FeatureCollection" {
		t.Errorf("expected type=FeatureCollection, got %v", parsed["type"])
	}
	features, ok := parsed["features"].([]interface{})
	if !ok || len(features) == 0 {
		t.Error("expected non-empty features array")
	}
}

// --- XLSX ---

func TestRenderXLSX_Array(t *testing.T) {
	data := []map[string]interface{}{
		{"adm_cd": "11", "adm_nm": "서울"},
		{"adm_cd": "26", "adm_nm": "부산"},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.xlsx")

	if err := Render(rawJSON(data), Options{Format: "xlsx", OutFile: out, Title: "행정구역"}); err != nil {
		t.Fatalf("Render xlsx array: %v", err)
	}

	// Verify file exists and is non-empty
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("xlsx file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("xlsx file is empty")
	}

	// Open and verify cell contents
	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("cannot open xlsx: %v", err)
	}
	defer wb.Close()

	// Check header cells (A1, B1 should be column names)
	a1, _ := wb.GetCellValue("행정구역", "A1")
	b1, _ := wb.GetCellValue("행정구역", "B1")
	if a1 == "" || b1 == "" {
		t.Errorf("expected non-empty header cells A1=%q B1=%q", a1, b1)
	}

	// Check that row 2 has data
	a2, _ := wb.GetCellValue("행정구역", "A2")
	b2, _ := wb.GetCellValue("행정구역", "B2")
	if a2 == "" && b2 == "" {
		t.Error("expected data in row 2 of xlsx")
	}
}

func TestRenderXLSX_Object(t *testing.T) {
	data := map[string]interface{}{"city": "Seoul", "code": "11"}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.xlsx")

	if err := Render(rawJSON(data), Options{Format: "xlsx", OutFile: out}); err != nil {
		t.Fatalf("Render xlsx object: %v", err)
	}

	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("cannot open xlsx: %v", err)
	}
	defer wb.Close()

	a1, _ := wb.GetCellValue("Sheet1", "A1")
	if a1 != "key" {
		t.Errorf("expected A1='key', got %q", a1)
	}
}

func TestRenderXLSX_RequiresOutFile(t *testing.T) {
	data := []map[string]interface{}{{"a": "1"}}
	err := Render(rawJSON(data), Options{Format: "xlsx"})
	if err == nil {
		t.Fatal("expected error when OutFile is empty for xlsx")
	}
}

// --- Null / empty result ---

func TestRender_NullResult(t *testing.T) {
	err := Render(json.RawMessage("null"), Options{Format: "json", OutFile: ""})
	if err != nil {
		t.Fatalf("unexpected error for null result: %v", err)
	}
}

func TestRender_EmptyResult(t *testing.T) {
	err := Render(json.RawMessage(""), Options{Format: "table"})
	if err != nil {
		t.Fatalf("unexpected error for empty result: %v", err)
	}
}

// --- Invalid format ---

func TestRender_InvalidFormat(t *testing.T) {
	err := Render(rawJSON("x"), Options{Format: "parquet"})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}
