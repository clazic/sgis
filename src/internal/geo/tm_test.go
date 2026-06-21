package geo

import (
	"math"
	"testing"
)

// TestEPSG5179ToWGS84_Anchor verifies the projection natural origin maps exactly
// to (lon=127.5, lat=38.0). This is an exact mathematical reference point.
func TestEPSG5179ToWGS84_Anchor(t *testing.T) {
	lon, lat := EPSG5179ToWGS84(tmFalseE, tmFalseN)
	if math.Abs(lon-127.5) > 1e-7 {
		t.Errorf("origin lon: got %.10f, want 127.5", lon)
	}
	if math.Abs(lat-38.0) > 1e-7 {
		t.Errorf("origin lat: got %.10f, want 38.0", lat)
	}
}

// TestEPSG5179ToWGS84_CentralMeridian checks that any point on the easting of
// the false origin (x=1,000,000) lies exactly on the central meridian (127.5°E),
// regardless of northing.
func TestEPSG5179ToWGS84_CentralMeridian(t *testing.T) {
	for _, y := range []float64{1800000, 1900000, 2000000, 2100000} {
		lon, _ := EPSG5179ToWGS84(tmFalseE, y)
		if math.Abs(lon-127.5) > 1e-7 {
			t.Errorf("central meridian at y=%.0f: lon got %.10f, want 127.5", y, lon)
		}
	}
}

// TestEPSG5179ToWGS84_Monotonic verifies easting↑ → lon↑ and northing↑ → lat↑.
func TestEPSG5179ToWGS84_Monotonic(t *testing.T) {
	lonW, _ := EPSG5179ToWGS84(900000, 1950000)
	lonE, _ := EPSG5179ToWGS84(1100000, 1950000)
	if !(lonE > lonW) {
		t.Errorf("easting↑ should increase lon: west=%.6f east=%.6f", lonW, lonE)
	}
	_, latS := EPSG5179ToWGS84(1000000, 1900000)
	_, latN := EPSG5179ToWGS84(1000000, 2100000)
	if !(latN > latS) {
		t.Errorf("northing↑ should increase lat: south=%.6f north=%.6f", latS, latN)
	}
}

// TestEPSG5179ToWGS84_KoreaRange sanity-checks a Seoul-area UTM-K coordinate
// lands within the Korean peninsula bounds.
func TestEPSG5179ToWGS84_KoreaRange(t *testing.T) {
	// Approximate UTM-K coordinates within the Seoul metropolitan area.
	lon, lat := EPSG5179ToWGS84(953900, 1952000)
	if lon < 124 || lon > 132 {
		t.Errorf("lon out of Korea range: %.6f", lon)
	}
	if lat < 33 || lat > 43 {
		t.Errorf("lat out of Korea range: %.6f", lat)
	}
}
