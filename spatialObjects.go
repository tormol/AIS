package AIS

import (
	"errors"
	"math"
)

/*POINT*/
type Point struct {
	lat  float64 //latitude, eg. 29.260799° N
	long float64 //longitude, eg. 94.87287° W
}

// Returns a point a's distance to another point b
func (a Point) DistanceTo(b Point) float64 {
	// [1.] Find the MBR
	aRect := Rectangle{max: a, min: a}
	mbr := aRect.MBRWith(&Rectangle{min: b, max: b})
	// [2.] Calculate the length of the diagonal
	length := math.Abs(mbr.max.long - mbr.min.long)
	height := math.Abs(mbr.max.lat - mbr.min.lat)
	var hypotenuse float64
	if length > 0 && height > 0 {
		hypotenuse = math.Sqrt(length*length + height*height) // Pythagoras: c^2 = a^2 + b^2
	} else {
		hypotenuse = math.Max(length, height) //if the length or the height of the MBR is zero, then the distance is given by the rectangle's longest side
	}
	return hypotenuse // [3.] end
}

/*RECTANGLE*/
type Rectangle struct {
	max Point //highest latitude, highest longitude
	min Point //lowest latitude, lowest longitude
}

// Returns a new Rectangle
func NewRectangle(minLat, minLong, maxLat, maxLong float64) (*Rectangle, error) {
	if minLat > maxLat || minLong > maxLong {
		return nil, errors.New("Error initializing Rectangle: min > max")
	} else if maxLat > 90 || minLat < -90 || maxLong > 180 || minLong < -180 {
		return nil, errors.New("Error initializing Rectangle: Illegal coordinates")
	}
	return &Rectangle{
		min: Point{
			lat:  minLat,
			long: minLong,
		},
		max: Point{
			lat:  maxLat,
			long: maxLong,
		},
	}, nil
}

// Returns the area of the Rectangle
func (a *Rectangle) Area() float64 { //TODO fix for Rectangles around the date line
	height := math.Abs(a.max.lat - a.min.lat)
	width := math.Abs(a.max.long - a.min.long)
	return height * width
}

// Returns the margin of the Rectangle
func (a *Rectangle) Margin() float64 {
	height := math.Abs(a.max.lat - a.min.lat)
	width := math.Abs(a.max.long - a.min.long)
	return 2 * (height + width)
}

// Returns the center point of the Rectangle
func (a *Rectangle) Center() Point {
	centerLat := a.min.lat + (math.Abs(a.max.lat-a.min.lat) / 2)
	centerLong := a.min.long + (math.Abs(a.max.long-a.min.long) / 2)
	return Point{lat: centerLat, long: centerLong}
}

// Does the Rectangle contatin a given point?
func (a *Rectangle) ContainsPoint(p Point) bool {
	r := false
	if p.lat >= a.min.lat && p.lat <= a.max.lat && p.long >= a.min.long && p.long <= a.max.long {
		r = true
	}
	return r
}

// Does Rectangle 'a' contain Rectangle 'b'?
func (a *Rectangle) ContainsRectangle(b *Rectangle) bool {
	r := false
	if a.ContainsPoint(b.min) && a.ContainsPoint(b.max) {
		r = true // If a contains both the min and the max point of b, then a contains b
	}
	return r
}

// Does rectangle 'a' and 'b' overlap?
func Overlaps(a, b *Rectangle) bool {
	r := true
	// Test if one of the rectangles is on the right side of the other
	if b.min.long > a.max.long || a.min.long > b.max.long {
		r = false
	}
	// Test if one of the rectangles is above the other
	if b.min.lat > a.max.lat || a.min.lat > b.max.lat {
		r = false
	}
	return r
}

// Returns the MBR containing both of the rectangles
func (a *Rectangle) MBRWith(r *Rectangle) *Rectangle {
	if a.ContainsRectangle(r) {
		return a
	} else {
		r, err := NewRectangle(math.Min(a.min.lat, r.min.lat), math.Min(a.min.long, r.min.long), math.Max(a.max.lat, r.max.lat), math.Max(a.max.long, r.max.long))
		CheckErr(err, "Error in the MBRWith func")
		return r
	}
}

// Returns the area of the overlapping area of the two rectangles
func (a *Rectangle) OverlapWith(b *Rectangle) float64 {
	if !Overlaps(a, b) {
		return 0
	} else if a.ContainsRectangle(b) {
		return b.Area()
	} else if b.ContainsRectangle(a) {
		return a.Area()
	}
	// find the overlapping rectangle's sides: the lowest "roof", the highest "floor, the rightmost "leftside", and the leftmost "rightside"
	leftside := a.min.long // gives the minLong
	if b.min.long > a.min.long {
		leftside = b.min.long
	}
	rightside := a.max.long // gives the maxLong
	if b.max.long < a.max.long {
		rightside = b.max.long
	}
	roof := a.max.lat // gives the maxLat
	if b.max.lat < a.max.lat {
		roof = b.max.lat
	}
	floor := a.min.lat //gives the minLat
	if b.min.lat > a.min.lat {
		floor = b.min.lat
	}
	// Make the Rectangle and return its area
	o, err := NewRectangle(floor, leftside, roof, rightside)
	CheckErr(err, "Failed to make the overlapping rectangle")
	return o.Area()
}

//Returns the difference in area between two rectangles
func (a *Rectangle) AreaDifference(b *Rectangle) float64 {
	return math.Abs(a.Area() - b.Area())
}

/*
Resources:
	https://blog.golang.org/go-maps-in-action	-	Structs containing simple objects can be used as map keys
*/
