package output

import (
	"encoding/json"
	"fmt"

	"github.com/clazic/sgis/internal/geo"
)

// renderGeoJSON passes the result through as valid GeoJSON (pretty-printed).
//
// SGIS boundary endpoints return coordinates in UTM-K / EPSG:5179 (Korean
// national projected CRS), NOT WGS84 (EPSG:4326). When opts.WGS84 is set the
// coordinate tree is reprojected to WGS84 (lon/lat) so consumers such as
// Leaflet can load the output directly; otherwise the raw UTM-K coordinates
// are preserved (no regression).
func renderGeoJSON(result json.RawMessage, opts Options) error {
	// Parse and re-encode to ensure valid JSON and normalised whitespace.
	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		return fmt.Errorf("GeoJSON 파싱 실패: %w", err)
	}

	if opts.WGS84 {
		v = reprojectTree(v)
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

// reprojectTree walks a decoded GeoJSON value and reprojects every "coordinates"
// array from EPSG:5179 to WGS84. A stale "bbox" (in source CRS units) is dropped
// to avoid misleading map consumers. All other keys are recursed into.
func reprojectTree(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, vv := range val {
			switch k {
			case "coordinates":
				val[k] = reprojectCoordArray(vv)
			case "bbox":
				delete(val, k)
			default:
				val[k] = reprojectTree(vv)
			}
		}
		return val
	case []interface{}:
		for i := range val {
			val[i] = reprojectTree(val[i])
		}
		return val
	}
	return v
}

// reprojectCoordArray recursively transforms a GeoJSON coordinate tree. A leaf
// is a numeric array [x, y, ...]; its first two elements are converted from
// UTM-K to WGS84 lon/lat and any extra elements (e.g. elevation) are preserved.
func reprojectCoordArray(v interface{}) interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return v
	}
	if len(arr) >= 2 {
		if x, xok := arr[0].(float64); xok {
			if y, yok := arr[1].(float64); yok {
				lon, lat := geo.EPSG5179ToWGS84(x, y)
				out := make([]interface{}, len(arr))
				out[0], out[1] = lon, lat
				copy(out[2:], arr[2:])
				return out
			}
		}
	}
	for i := range arr {
		arr[i] = reprojectCoordArray(arr[i])
	}
	return arr
}
