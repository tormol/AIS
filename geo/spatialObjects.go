package geo

import (
	"errors"
	"log"
	"math"
)

/*POINT*/
type Point struct {
	Lat  float64 //latitude, eg. 29.260799° N
	Long float64 //longitude, eg. 94.87287° W
}

// Returns a point a's distance to another point b
func (a Point) DistanceTo(b Point) float64 {
	// [1.] Find the MBR
	aRect := Rectangle{Max: a, Min: a}
	mbr := aRect.MBRWith(&Rectangle{Min: b, Max: b})
	// [2.] Calculate the length of the diagonal
	length := math.Abs(mbr.Max.Long - mbr.Min.Long)
	height := math.Abs(mbr.Max.Lat - mbr.Min.Lat)
	var hypotenuse float64
	if length > 0 && height > 0 {
		hypotenuse = math.Sqrt(length*length + height*height) // Pythagoras: c^2 = a^2 + b^2
	} else {
		hypotenuse = math.Max(length, height) //if the length or the height of the MBR is zero, then the distance is given by the rectangle's longest side
	}
	return hypotenuse // [3.] end
}

// Returns true if the coordinates are legal
func LegalCoord(lat, long float64) bool {
	if lat > 90 || lat < -90 || long > 180 || long < -180 {
		return false
	}
	return true
}

/*RECTANGLE*/
type Rectangle struct {
	Max Point //highest latitude, highest longitude
	Min Point //lowest latitude, lowest longitude
}

// Returns a new Rectangle
func NewRectangle(minLat, minLong, maxLat, maxLong float64) (*Rectangle, error) {
	if minLat > maxLat || minLong > maxLong {
		return nil, errors.New("Error initializing Rectangle: min > max")
	} else if !LegalCoord(minLat, minLong) || !LegalCoord(maxLat, maxLong) {
		return nil, errors.New("Error initializing Rectangle: Illegal coordinates")
	}
	return &Rectangle{
		Min: Point{
			Lat:  minLat,
			Long: minLong,
		},
		Max: Point{
			Lat:  maxLat,
			Long: maxLong,
		},
	}, nil
}

// Returns the area of the Rectangle
func (a *Rectangle) Area() float64 { //TODO fix for Rectangles around the date line
	height := math.Abs(a.Max.Lat - a.Min.Lat)
	width := math.Abs(a.Max.Long - a.Min.Long)
	return height * width
}

// Returns the margin of the Rectangle
func (a *Rectangle) Margin() float64 {
	height := math.Abs(a.Max.Lat - a.Min.Lat)
	width := math.Abs(a.Max.Long - a.Min.Long)
	return 2 * (height + width)
}

// Returns the center point of the Rectangle
func (a *Rectangle) Center() Point {
	centerLat := a.Min.Lat + (math.Abs(a.Max.Lat-a.Min.Lat) / 2)
	centerLong := a.Min.Long + (math.Abs(a.Max.Long-a.Min.Long) / 2)
	return Point{Lat: centerLat, Long: centerLong}
}

// Does the Rectangle contatin a given point?
func (a *Rectangle) ContainsPoint(p Point) bool {
	r := false
	if p.Lat >= a.Min.Lat && p.Lat <= a.Max.Lat && p.Long >= a.Min.Long && p.Long <= a.Max.Long {
		r = true
	}
	return r
}

// Does Rectangle 'a' contain Rectangle 'b'?
func (a *Rectangle) ContainsRectangle(b *Rectangle) bool {
	r := false
	if a.ContainsPoint(b.Min) && a.ContainsPoint(b.Max) {
		r = true // If a contains both the min and the max point of b, then a contains b
	}
	return r
}

// Does rectangle 'a' and 'b' overlap?
func Overlaps(a, b *Rectangle) bool {
	r := true
	// Test if one of the rectangles is on the right side of the other
	if b.Min.Long > a.Max.Long || a.Min.Long > b.Max.Long {
		r = false
	}
	// Test if one of the rectangles is above the other
	if b.Min.Lat > a.Max.Lat || a.Min.Lat > b.Max.Lat {
		r = false
	}
	return r
}

// Returns the MBR containing both of the rectangles
func (a *Rectangle) MBRWith(r *Rectangle) *Rectangle {
	if a.ContainsRectangle(r) {
		return a
	} else {
		r, err := NewRectangle(math.Min(a.Min.Lat, r.Min.Lat), math.Min(a.Min.Long, r.Min.Long), math.Max(a.Max.Lat, r.Max.Lat), math.Max(a.Max.Long, r.Max.Long))
		if err != nil {
			log.Println("Failed to calculate MBR of two rectangles...")
			return nil
		}
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
	leftside := a.Min.Long // gives the minLong
	if b.Min.Long > a.Min.Long {
		leftside = b.Min.Long
	}
	rightside := a.Max.Long // gives the maxLong
	if b.Max.Long < a.Max.Long {
		rightside = b.Max.Long
	}
	roof := a.Max.Lat // gives the maxLat
	if b.Max.Lat < a.Max.Lat {
		roof = b.Max.Lat
	}
	floor := a.Min.Lat //gives the minLat
	if b.Min.Lat > a.Min.Lat {
		floor = b.Min.Lat
	}
	// Make the Rectangle and return its area
	o, err := NewRectangle(floor, leftside, roof, rightside)
	if err != nil {
		log.Println("Error[!] cannot calculate the overlap of the two rectangles")
	}
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
