// Package geo provides coordinate reprojection for SGIS spatial data.
//
// SGIS boundary endpoints return coordinates in UTM-K (EPSG:5179, Korea 2000 /
// Unified Coordinate System), a single Transverse Mercator projection on the
// GRS80 ellipsoid. This package converts those projected (x, y) metre
// coordinates to WGS84 (EPSG:4326) longitude/latitude so the output can be
// consumed directly by Leaflet and other web mapping libraries.
//
// Implementation is pure Go (no CGO / PROJ dependency) using the standard
// Transverse Mercator inverse formulas, which keeps the cross-platform
// CGO_ENABLED=0 build invariant intact.
package geo

import "math"

// EPSG:5179 projection parameters (Korea 2000 / Unified CS, GRS80 ellipsoid).
const (
	grs80A   = 6378137.0           // semi-major axis (m)
	grs80F   = 1.0 / 298.257222101 // flattening
	tmK0     = 0.9996              // scale factor at central meridian
	tmLon0   = 127.5              // central meridian (degrees E)
	tmLat0   = 38.0              // latitude of origin (degrees N)
	tmFalseE = 1000000.0          // false easting (m)
	tmFalseN = 2000000.0          // false northing (m)

	deg2rad = math.Pi / 180.0
	rad2deg = 180.0 / math.Pi
)

// meridianArc returns the meridional arc length M from the equator to latitude
// phi (radians) on an ellipsoid with eccentricity-squared e2 and semi-major a.
func meridianArc(phi, a, e2 float64) float64 {
	return a * ((1-e2/4-3*e2*e2/64-5*e2*e2*e2/256)*phi -
		(3*e2/8+3*e2*e2/32+45*e2*e2*e2/1024)*math.Sin(2*phi) +
		(15*e2*e2/256+45*e2*e2*e2/1024)*math.Sin(4*phi) -
		(35*e2*e2*e2/3072)*math.Sin(6*phi))
}

// EPSG5179ToWGS84 converts UTM-K (EPSG:5179) projected coordinates (x easting,
// y northing, in metres) to WGS84 geographic coordinates, returning longitude
// and latitude in degrees.
//
// Anchor: the projection's natural origin (x=1000000, y=2000000) maps exactly
// to (lon=127.5, lat=38.0), which the unit tests use as a precise reference.
func EPSG5179ToWGS84(x, y float64) (lon, lat float64) {
	a := grs80A
	e2 := grs80F * (2 - grs80F) // first eccentricity squared
	ep2 := e2 / (1 - e2)        // second eccentricity squared

	lat0 := tmLat0 * deg2rad
	lon0 := tmLon0 * deg2rad

	// Footpoint latitude from the (de-offset, de-scaled) northing.
	m0 := meridianArc(lat0, a, e2)
	m := m0 + (y-tmFalseN)/tmK0

	mu := m / (a * (1 - e2/4 - 3*e2*e2/64 - 5*e2*e2*e2/256))
	e1 := (1 - math.Sqrt(1-e2)) / (1 + math.Sqrt(1-e2))

	j1 := 3*e1/2 - 27*e1*e1*e1/32
	j2 := 21*e1*e1/16 - 55*e1*e1*e1*e1/32
	j3 := 151 * e1 * e1 * e1 / 96
	j4 := 1097 * e1 * e1 * e1 * e1 / 512

	fp := mu + j1*math.Sin(2*mu) + j2*math.Sin(4*mu) + j3*math.Sin(6*mu) + j4*math.Sin(8*mu)

	sinFp := math.Sin(fp)
	cosFp := math.Cos(fp)
	tanFp := sinFp / cosFp

	c1 := ep2 * cosFp * cosFp
	t1 := tanFp * tanFp
	sinFp2 := sinFp * sinFp
	r1 := a * (1 - e2) / math.Pow(1-e2*sinFp2, 1.5)
	n1 := a / math.Sqrt(1-e2*sinFp2)
	d := (x - tmFalseE) / (n1 * tmK0)

	// Latitude.
	q1 := n1 * tanFp / r1
	q2 := d * d / 2
	q3 := (5 + 3*t1 + 10*c1 - 4*c1*c1 - 9*ep2) * math.Pow(d, 4) / 24
	q4 := (61 + 90*t1 + 298*c1 + 45*t1*t1 - 3*c1*c1 - 252*ep2) * math.Pow(d, 6) / 720
	latRad := fp - q1*(q2-q3+q4)

	// Longitude.
	q5 := d
	q6 := (1 + 2*t1 + c1) * math.Pow(d, 3) / 6
	q7 := (5 - 2*c1 + 28*t1 - 3*c1*c1 + 8*ep2 + 24*t1*t1) * math.Pow(d, 5) / 120
	lonRad := lon0 + (q5-q6+q7)/cosFp

	return lonRad * rad2deg, latRad * rad2deg
}
