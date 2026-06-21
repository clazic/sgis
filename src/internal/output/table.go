package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// renderTable renders result as an aligned text table.
// - Array of flat objects → column table (with Fields projection).
// - Single flat object   → 2-column key/value table.
// - Nested/complex       → compact JSON fallback with note.
func renderTable(result json.RawMessage, isArray bool, opts Options) error {
	f, close, err := openOutput(opts.OutFile)
	if err != nil {
		return err
	}
	defer close()

	if isArray {
		rows, err := parseArray(result)
		if err != nil {
			return fmt.Errorf("배열 파싱 실패: %w", err)
		}
		if len(rows) == 0 {
			fmt.Fprintln(f, "(데이터 없음)")
			return nil
		}
		// Check flatness using first row
		if len(rows) > 0 && !isFlat(rows[0]) {
			return renderCompactFallback(result, f)
		}
		return renderArrayTable(rows, opts.Fields, f)
	}

	// Single object
	obj, err := parseObject(result)
	if err != nil {
		return fmt.Errorf("객체 파싱 실패: %w", err)
	}
	if !isFlat(obj) {
		return renderCompactFallback(result, f)
	}
	return renderKeyValueTable(obj, opts.Fields, f)
}

// renderArrayTable prints an aligned table from a slice of flat maps.
func renderArrayTable(rows []map[string]interface{}, fields []string, f interface{ WriteString(string) (int, error) }) error {
	// Determine columns from first row
	cols := projectFields(rows[0], fields)
	if len(cols) == 0 {
		return nil
	}

	// Calculate column widths (header width vs max data width)
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, row := range rows {
		for i, c := range cols {
			if l := len(formatValue(row[c])); l > widths[i] {
				widths[i] = l
			}
		}
	}
	// Cap each column at 60 chars
	for i := range widths {
		if widths[i] > 60 {
			widths[i] = 60
		}
	}

	var buf bytes.Buffer
	sep := buildSeparator(widths)

	// Top border
	buf.WriteString(sep)
	buf.WriteByte('\n')

	// Header row
	buf.WriteString("│")
	for i, c := range cols {
		buf.WriteString(fmt.Sprintf(" %-*s │", widths[i], truncate(c, widths[i])))
	}
	buf.WriteByte('\n')

	// Header / data separator
	buf.WriteString(buildMidSeparator(widths))
	buf.WriteByte('\n')

	// Data rows
	for _, row := range rows {
		buf.WriteString("│")
		for i, c := range cols {
			val := truncate(formatValue(row[c]), widths[i])
			buf.WriteString(fmt.Sprintf(" %-*s │", widths[i], val))
		}
		buf.WriteByte('\n')
	}

	// Bottom border
	buf.WriteString(buildBottomSeparator(widths))
	buf.WriteByte('\n')

	_, err := f.WriteString(buf.String())
	return err
}

// renderKeyValueTable prints a 2-column key/value table for a single object.
func renderKeyValueTable(obj map[string]interface{}, fields []string, f interface{ WriteString(string) (int, error) }) error {
	keys := projectFields(obj, fields)
	if len(keys) == 0 {
		return nil
	}

	keyW := len("키")
	valW := len("값")
	for _, k := range keys {
		if l := len(k); l > keyW {
			keyW = l
		}
		if l := len(formatValue(obj[k])); l > valW {
			valW = l
		}
	}
	if keyW > 40 {
		keyW = 40
	}
	if valW > 60 {
		valW = 60
	}

	widths := []int{keyW, valW}
	var buf bytes.Buffer

	buf.WriteString(buildSeparator(widths))
	buf.WriteByte('\n')
	buf.WriteString(fmt.Sprintf("│ %-*s │ %-*s │\n", keyW, "키", valW, "값"))
	buf.WriteString(buildMidSeparator(widths))
	buf.WriteByte('\n')

	for _, k := range keys {
		kStr := truncate(k, keyW)
		vStr := truncate(formatValue(obj[k]), valW)
		buf.WriteString(fmt.Sprintf("│ %-*s │ %-*s │\n", keyW, kStr, valW, vStr))
	}

	buf.WriteString(buildBottomSeparator(widths))
	buf.WriteByte('\n')

	_, err := f.WriteString(buf.String())
	return err
}

// renderCompactFallback prints compact JSON with a note when structure is nested.
func renderCompactFallback(result json.RawMessage, f interface{ WriteString(string) (int, error) }) error {
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return err
	}
	out, err := json.Marshal(v)
	if err != nil {
		return err
	}
	note := "// NOTE: 중첩 구조로 인해 table 렌더링 불가 — compact JSON으로 출력합니다. --format json 사용을 권장합니다.\n"
	_, err = f.WriteString(note + string(out) + "\n")
	return err
}

// --- separator helpers ---

func buildSeparator(widths []int) string {
	var b strings.Builder
	b.WriteString("┌")
	for i, w := range widths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < len(widths)-1 {
			b.WriteString("┬")
		}
	}
	b.WriteString("┐")
	return b.String()
}

func buildMidSeparator(widths []int) string {
	var b strings.Builder
	b.WriteString("├")
	for i, w := range widths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < len(widths)-1 {
			b.WriteString("┼")
		}
	}
	b.WriteString("┤")
	return b.String()
}

func buildBottomSeparator(widths []int) string {
	var b strings.Builder
	b.WriteString("└")
	for i, w := range widths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < len(widths)-1 {
			b.WriteString("┴")
		}
	}
	b.WriteString("┘")
	return b.String()
}

// truncate clips s to maxLen bytes (ASCII-safe; SGIS keys are ASCII).
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 2 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}
