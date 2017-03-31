package geo

import (
	"encoding/json"
	"errors"
	"log"
	"math"
)

// Point is a set of <latitude, longitude> coordinates.
type Point struct {
	Lat  float64 //latitude, eg. 29.260799° N
	Long float64 //longitude, eg. 94.87287° W
}

// DistanceTo returns the point's distance to another point.
func (a Point) DistanceTo(b Point) float64 {
	// [1.] Find the MBR
	aRect := Rectangle{max: a, min: a}
	mbr := aRect.MBRWith(&Rectangle{min: b, max: b})
	// [2.] Calculate the length of the diagonal
	length := math.Abs(mbr.max.Long - mbr.min.Long)
	height := math.Abs(mbr.max.Lat - mbr.min.Lat)
	var hypotenuse float64
	if length > 0 && height > 0 {
		hypotenuse = math.Sqrt(length*length + height*height) // Pythagoras: c^2 = a^2 + b^2
	} else {
		hypotenuse = math.Max(length, height) //if the length or the height of the MBR is zero, then the distance is given by the rectangle's longest side
	}
	return hypotenuse // [3.] end
}

// MarshalJSON returns the GeoJSON representation of the coordinates.
func (p Point) MarshalJSON() ([]byte, error) {
	return json.Marshal([]float64{p.Long, p.Lat})
}

// UnmarshalJSON unmarshals the JSON-data in b to a Point-object.
func (p *Point) UnmarshalJSON(b []byte) error {
	var s []float64
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if len(s) != 2 {
		return errors.New("Wrong dimensionality of the Point")
	}
	p.Long = s[0]
	p.Lat = s[1]
	return nil
}

// LegalCoord returns true if the given coordinates are legal.
// lat=-90 and lon=-180 are permitted because they're useful in search rectangles.
func LegalCoord(lat, long float64) bool {
	return lat <= 90.0 && lat >= -90.0 && long <= 180.0 && long >= -180.0
}

// Rectangle consists of two <lat,long> Points.
// "max" contains the point with the highest latitude and the hightest longitude
// "min" contains the point with the lowest latitude and the lowest longitude
type Rectangle struct {
	max Point
	min Point
}

// NewRectangle returns a pointer to a new Rectangle.
func NewRectangle(minLat, minLong, maxLat, maxLong float64) (*Rectangle, error) {
	if minLat > maxLat || minLong > maxLong {
		return nil, errors.New("Error initializing Rectangle: min > max")
	} else if !LegalCoord(minLat, minLong) || !LegalCoord(maxLat, maxLong) {
		return nil, errors.New("Error initializing Rectangle: Illegal coordinates")
	}
	return &Rectangle{
		min: Point{
			Lat:  minLat,
			Long: minLong,
		},
		max: Point{
			Lat:  maxLat,
			Long: maxLong,
		},
	}, nil
}

// Max returns the highest (most north-eastern) <lat,long> Point of the rectangle.
func (a *Rectangle) Max() Point { return a.max }

// Min returns the lowest (most south-western) <lat,long> Point of the rectangle.
func (a *Rectangle) Min() Point { return a.min }

// Area returns the area of the rectangle.
func (a *Rectangle) Area() float64 {
	height := math.Abs(a.max.Lat - a.min.Lat)
	width := math.Abs(a.max.Long - a.min.Long)
	return height * width
}

// Margin returns the margin of the rectangle.
func (a *Rectangle) Margin() float64 {
	height := math.Abs(a.max.Lat - a.min.Lat)
	width := math.Abs(a.max.Long - a.min.Long)
	return 2 * (height + width)
}

// Center returns the center point of the Rectangle.
func (a *Rectangle) Center() Point {
	centerLat := a.min.Lat + (math.Abs(a.max.Lat-a.min.Lat) / 2)
	centerLong := a.min.Long + (math.Abs(a.max.Long-a.min.Long) / 2)
	return Point{Lat: centerLat, Long: centerLong}
}

// ContainsPoint checks if the Rectangle contains a given point.
func (a *Rectangle) ContainsPoint(p Point) bool {
	return p.Lat >= a.min.Lat && p.Lat <= a.max.Lat &&
		p.Long >= a.min.Long && p.Long <= a.max.Long
}

// ContainsRectangle checks if the Rectangle contains a given other Rectangle.
func (a *Rectangle) ContainsRectangle(b *Rectangle) bool {
	// If a contains both the min and the max point of b, then a contains b
	return a.ContainsPoint(b.min) && a.ContainsPoint(b.max)
}

// Overlaps checks if rectangle 'a' and 'b' are overlapping.
// Returns true for rectangles that just touch (==).
func Overlaps(a, b *Rectangle) bool {
	return b.min.Long <= a.max.Long && a.min.Long <= b.max.Long && // Test if one of the rectangles is on the right side of the other
		b.min.Lat <= a.max.Lat && a.min.Lat <= b.max.Lat // Test if one of the rectangles is above the other
}

// MBRWith returns the minimum bounding rectangle which contains both of the rectangles.
func (a *Rectangle) MBRWith(r *Rectangle) *Rectangle {
	if a.ContainsRectangle(r) {
		return a
	}
	r, err := NewRectangle(math.Min(a.min.Lat, r.min.Lat), math.Min(a.min.Long, r.min.Long), math.Max(a.max.Lat, r.max.Lat), math.Max(a.max.Long, r.max.Long))
	if err != nil {
		log.Println("Failed to calculate MBR of two rectangles...")
		return nil
	}
	return r
}

// OverlapWith returns the area of the overlapping area of the two rectangles.
func (a *Rectangle) OverlapWith(b *Rectangle) float64 {
	if !Overlaps(a, b) {
		return 0
	} else if a.ContainsRectangle(b) {
		return b.Area()
	} else if b.ContainsRectangle(a) {
		return a.Area()
	}
	// find the overlapping rectangle's sides: the lowest "roof", the highest "floor, the rightmost "leftside", and the leftmost "rightside"
	leftside := a.min.Long // gives the minLong
	if b.min.Long > a.min.Long {
		leftside = b.min.Long
	}
	rightside := a.max.Long // gives the maxLong
	if b.max.Long < a.max.Long {
		rightside = b.max.Long
	}
	roof := a.max.Lat // gives the maxLat
	if b.max.Lat < a.max.Lat {
		roof = b.max.Lat
	}
	floor := a.min.Lat //gives the minLat
	if b.min.Lat > a.min.Lat {
		floor = b.min.Lat
	}
	// Make the Rectangle and return its area
	o, err := NewRectangle(floor, leftside, roof, rightside)
	if err != nil {
		log.Println("Error[!] cannot calculate the overlap of the two rectangles")
	}
	return o.Area()
}

// AreaDifference returns the difference in area between two rectangles.
func (a *Rectangle) AreaDifference(b *Rectangle) float64 {
	return math.Abs(a.Area() - b.Area())
}

// SplitViewRect maps any rectangular view of the earth to a set of
// non-overlapping, valid rectangles.
// More than one rectangle is needed if the view crosses the date line
// or a pole. (the latter is'nt supported yet)
func SplitViewRect(minLat, minLong, maxLat, maxLong float64) []Rectangle {
	// reject troublesome special values
	for _, f := range [...]float64{minLat, minLong, maxLat, maxLong} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
	}
	if maxLong-minLong >= 360.0 {
		// all longtitudes
		minLong = -180
		maxLong = 180
	} else {
		// move
		for minLong < -180.0 {
			minLong += 360.0
		}
		for minLong > 180.0 {
			minLong -= 360.0
		}
		for maxLong < -180.0 {
			maxLong += 360.0
		}
		for maxLong > 180.0 {
			maxLong -= 360.0
		}
	}

	if maxLat < minLat || maxLat < -90.0 || minLat > 90 {
		return nil // doesn't make sense to wrap from one pole to another
	}
	if maxLat-minLat >= 180.0 {
		// all latitudes
		minLat = -90
		maxLat = 90
	}

	if maxLong >= minLong && minLat >= -90.0 && maxLat <= 90.0 {
		// single
		return []Rectangle{Rectangle{
			min: Point{minLat, minLong},
			max: Point{maxLat, maxLong},
		}}
	} else if maxLong < minLong && minLat >= -90.0 && maxLat <= 90.0 {
		return []Rectangle{
			Rectangle{min: Point{minLat, -180.0}, max: Point{maxLat, maxLong}}, // west
			Rectangle{min: Point{minLat, minLong}, max: Point{maxLat, 180.0}},  // east
		}
	}
	return nil // TODO mirroring around poles (not encountered from the web view)
	// if math.Abs(maxLon-minLon) > 45.0 we must be careful to avoid overlapping
	// if the date line is visible a horizontal split isn't enough, so drop that
	// above/below the latitude closest to the pole all latitudes are visible,
	// and then there is a smaller rectangle next to it with height equal to the
	// reflected difference in latitude.
}

/*
Resources:
	https://blog.golang.org/go-maps-in-action	-	Structs containing simple objects can be used as map keys
*/
