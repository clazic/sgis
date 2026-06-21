package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
)

// renderCSV formats result as CSV.
// - Array of objects → header row + data rows (Fields projection applied).
// - Single object    → 2-column key,value CSV.
func renderCSV(result json.RawMessage, isArray bool, opts Options) error {
	f, close, err := openOutput(opts.OutFile)
	if err != nil {
		return err
	}
	defer close()

	w := csv.NewWriter(f)

	if isArray {
		rows, err := parseArray(result)
		if err != nil {
			return fmt.Errorf("배열 파싱 실패: %w", err)
		}
		if len(rows) == 0 {
			w.Flush()
			return w.Error()
		}

		cols := projectFields(rows[0], opts.Fields)

		// Header row
		if err := w.Write(cols); err != nil {
			return fmt.Errorf("CSV 헤더 쓰기 실패: %w", err)
		}

		// Data rows
		for i, row := range rows {
			record := make([]string, len(cols))
			for j, c := range cols {
				record[j] = sanitizeCSV(formatValue(row[c]))
			}
			if err := w.Write(record); err != nil {
				return fmt.Errorf("CSV 행 %d 쓰기 실패: %w", i+1, err)
			}
		}
	} else {
		// Single object → 2-col key,value
		obj, err := parseObject(result)
		if err != nil {
			return fmt.Errorf("객체 파싱 실패: %w", err)
		}

		// Header
		if err := w.Write([]string{"key", "value"}); err != nil {
			return fmt.Errorf("CSV 헤더 쓰기 실패: %w", err)
		}

		keys := projectFields(obj, opts.Fields)
		for _, k := range keys {
			if err := w.Write([]string{k, sanitizeCSV(formatValue(obj[k]))}); err != nil {
				return fmt.Errorf("CSV 행 쓰기 실패: %w", err)
			}
		}
	}

	w.Flush()
	return w.Error()
}

// sanitizeCSV prevents formula injection when the CSV is opened in spreadsheet apps.
func sanitizeCSV(s string) string {
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "=") || strings.HasPrefix(s, "+") ||
		strings.HasPrefix(s, "-") || strings.HasPrefix(s, "@") ||
		strings.HasPrefix(s, "\t") || strings.HasPrefix(s, "\r") {
		return "'" + s
	}
	return s
}
