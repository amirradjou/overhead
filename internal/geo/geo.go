// Package geo has the little spherical geometry overhead needs: distance and bearing from home.
package geo

import "math"

// KmPerNM converts nautical miles, the unit the flight-tracking APIs use for radius, to kilometres.
const KmPerNM = 1.852

// earthRadiusKm is the mean Earth radius; at 30 km the spherical error is well under the feeds' position error.
const earthRadiusKm = 6371.0088

// Point is a WGS84 latitude and longitude in degrees.
type Point struct {
	Lat, Lon float64
}

// Valid reports whether p is a real coordinate.
func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

// DistanceKm is the great-circle (haversine) distance between a and b.
func DistanceKm(a, b Point) float64 {
	lat1, lat2 := rad(a.Lat), rad(b.Lat)
	dLat, dLon := lat2-lat1, rad(b.Lon-a.Lon)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

// BearingDeg is the initial compass bearing from one point to another, 0 (north) up to 360.
func BearingDeg(from, to Point) float64 {
	lat1, lat2 := rad(from.Lat), rad(to.Lat)
	dLon := rad(to.Lon - from.Lon)
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }
