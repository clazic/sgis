package output

import (
	"encoding/json"
	"fmt"
)

// renderJSON pretty-prints the raw result payload (indent 2 spaces).
func renderJSON(result json.RawMessage, opts Options) error {
	// Re-indent: unmarshal then marshal with indent
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return fmt.Errorf("JSON 파싱 실패: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 직렬화 실패: %w", err)
	}

	f, close, err := openOutput(opts.OutFile)
	if err != nil {
		return err
	}
	defer close()

	_, err = fmt.Fprintf(f, "%s\n", out)
	return err
}
