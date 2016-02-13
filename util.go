package main

import (
	"math"

	"github.com/kellydunn/golang-geo"
)

func tileToPoint(x, y, z int) *geo.Point {
	xf := float64(x)
	yf := float64(y)
	zf := float64(z)

	long := xf/math.Pow(2, zf)*360 - 180
	n := math.Pi - 2*math.Pi*yf/math.Pow(2, zf)
	lat := (180 / math.Pi * math.Atan(0.5*(math.Exp(n)-math.Exp(-n))))

	return geo.NewPoint(lat, long)
}
