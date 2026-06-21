package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func geoJSONWithPoint(x, y float64) json.RawMessage {
	fc := map[string]interface{}{
		"type": "FeatureCollection",
		"bbox": []interface{}{x, y, x, y},
		"features": []interface{}{
			map[string]interface{}{
				"type": "Feature",
				"geometry": map[string]interface{}{
					"type":        "Point",
					"coordinates": []interface{}{x, y},
				},
				"properties": map[string]interface{}{"name": "test"},
			},
		},
	}
	b, _ := json.Marshal(fc)
	return b
}

func TestRenderGeoJSON_WGS84Reproject(t *testing.T) {
	// EPSG:5179 natural origin → exactly (127.5, 38.0).
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.geojson")
	if err := Render(geoJSONWithPoint(1000000, 2000000), Options{Format: "geojson", OutFile: out, WGS84: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content, _ := os.ReadFile(out)
	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, hasBbox := parsed["bbox"]; hasBbox {
		t.Error("stale bbox should be dropped on WGS84 reprojection")
	}
	feat := parsed["features"].([]interface{})[0].(map[string]interface{})
	coords := feat["geometry"].(map[string]interface{})["coordinates"].([]interface{})
	lon := coords[0].(float64)
	lat := coords[1].(float64)
	if lon < 127.49 || lon > 127.51 {
		t.Errorf("lon not reprojected: %.6f", lon)
	}
	if lat < 37.99 || lat > 38.01 {
		t.Errorf("lat not reprojected: %.6f", lat)
	}
}

func TestRenderGeoJSON_NoWGS84Regression(t *testing.T) {
	// Without WGS84, coordinates must be byte-identical passthrough.
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.geojson")
	if err := Render(geoJSONWithPoint(1000000, 2000000), Options{Format: "geojson", OutFile: out}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content, _ := os.ReadFile(out)
	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	feat := parsed["features"].([]interface{})[0].(map[string]interface{})
	coords := feat["geometry"].(map[string]interface{})["coordinates"].([]interface{})
	if coords[0].(float64) != 1000000 || coords[1].(float64) != 2000000 {
		t.Errorf("coordinates altered without WGS84: %v", coords)
	}
}
