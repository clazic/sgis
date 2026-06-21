package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/text/width"
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

	// Calculate column widths (header width vs max data width, display-width based)
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = dispWidth(c)
	}
	for _, row := range rows {
		for i, c := range cols {
			if l := dispWidth(formatValue(row[c])); l > widths[i] {
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
		buf.WriteString(" " + padRight(truncate(c, widths[i]), widths[i]) + " │")
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
			buf.WriteString(" " + padRight(val, widths[i]) + " │")
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

	keyW := dispWidth("키")
	valW := dispWidth("값")
	for _, k := range keys {
		if l := dispWidth(k); l > keyW {
			keyW = l
		}
		if l := dispWidth(formatValue(obj[k])); l > valW {
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
	buf.WriteString("│ " + padRight("키", keyW) + " │ " + padRight("값", valW) + " │\n")
	buf.WriteString(buildMidSeparator(widths))
	buf.WriteByte('\n')

	for _, k := range keys {
		kStr := truncate(k, keyW)
		vStr := truncate(formatValue(obj[k]), valW)
		buf.WriteString("│ " + padRight(kStr, keyW) + " │ " + padRight(vStr, valW) + " │\n")
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

// runeWidth returns the terminal display width of a single rune:
// 2 for East Asian wide/fullwidth (e.g. Hangul, CJK), 1 otherwise.
func runeWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}

// dispWidth returns the total terminal display width of s, accounting for
// East Asian fullwidth characters (counted as 2 columns each).
func dispWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// padRight pads s with spaces on the right so its display width equals width.
// Replaces fmt's %-*s, which pads by byte count and misaligns fullwidth text.
func padRight(s string, w int) string {
	pad := w - dispWidth(s)
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

// truncate clips s so its display width does not exceed maxWidth, appending
// ".." (width 2) when clipped. Operates on runes so multibyte characters are
// never split mid-byte.
func truncate(s string, maxWidth int) string {
	if dispWidth(s) <= maxWidth {
		return s
	}
	// Not enough room for an ellipsis: fill up to maxWidth by display width.
	if maxWidth <= 2 {
		var b strings.Builder
		w := 0
		for _, r := range s {
			rw := runeWidth(r)
			if w+rw > maxWidth {
				break
			}
			b.WriteRune(r)
			w += rw
		}
		return b.String()
	}
	limit := maxWidth - 2 // reserve 2 columns for ".."
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runeWidth(r)
		if w+rw > limit {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + ".."
}
