package output

import (
	"encoding/json"
	"fmt"
)

// renderGeoJSON passes the result through as valid GeoJSON (pretty-printed).
//
// NOTE: SGIS boundary endpoints return coordinates in UTM-K / EPSG:5179 (Korean
// national projected CRS), NOT WGS84 (EPSG:4326). Reprojection to WGS84 is the
// responsibility of the map template layer (Phase 7 / chart/map.go), not here.
// Consumers that need WGS84 (e.g. Leaflet maps) must reproject before rendering.
func renderGeoJSON(result json.RawMessage, opts Options) error {
	// Parse and re-encode to ensure valid JSON and normalised whitespace.
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return fmt.Errorf("GeoJSON 파싱 실패: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("GeoJSON 직렬화 실패: %w", err)
	}

	f, close, err := openOutput(opts.OutFile)
	if err != nil {
		return err
	}
	defer close()

	_, err = fmt.Fprintf(f, "%s\n", out)
	return err
}
