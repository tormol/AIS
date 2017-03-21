package geo

import (
	"testing"
)

func TestDistanceTo(t *testing.T) {
	cases := []struct {
		a, b     Point
		expected float64
	}{
		{Point{0, 0}, Point{0, 0}, 0.0},
		{Point{80, 0}, Point{0, 0}, 80.0},
		{Point{0, 0}, Point{1, 1}, 1.4142135623730951},
		{Point{-0, -0}, Point{-1, -1}, 1.4142135623730951},
		{Point{1, -1}, Point{0, 0}, 1.4142135623730951},
		{Point{79.999999, 179.999999}, Point{-79.999999, -179.999999}, 393.9543094319442},
	}
	for _, c := range cases {
		dist := c.a.DistanceTo(c.b)
		if dist != c.expected {
			t.Log("ERROR, should be ", c.expected, " got ", dist) //print message to screen
			t.Fail()                                              //indicates that the test failed
		}
	}
}

func TestLegalCoords(t *testing.T) {
	cases := []struct {
		lat, long float64
		expected  bool
	}{
		{0, 0, true},
		{-0, -0, true},
		{90, 180, true},
		{-90, -180, true},
		{-90.000001, -180, false},
		{90.000001, -180, false},
		{90, -180.000001, false},
		{90, 180.000001, false},
		{179, 79, false},
	}
	for _, c := range cases {
		r := LegalCoord(c.lat, c.long)
		if r != c.expected {
			t.Log("ERROR, should be ", c.expected, " got ", r, " for coordinates ", c.lat, ",", c.long)
			t.Fail()
		}
	}
}

func TestNewRectangle(t *testing.T) {
	cases := []struct {
		minlat, minlong, maxlat, maxlong float64
		expectedFail                     bool
	}{
		{0, 0, 0, 0, false},
		{-1.987654, -1.123456, -0.123456, 0.123456, false},
		{-90, -180, 90, 180, false},
		{0, 0, -0.01, 0, true},
		{9000, 0, 0, 0, true},
		{0, 0, 0, -1, true},
	}
	for _, c := range cases {
		rect, err := NewRectangle(c.minlat, c.minlong, c.maxlat, c.maxlong)
		if err != nil && !c.expectedFail { //unexpected error
			t.Log("err is not nil, ", err)
			t.Fail()
		} else if rect != nil {
			if c.expectedFail { //didnt trigger error
				t.Log("should not return a rectangle, but did")
				t.Fail()
			} else if rect.min.Lat != c.minlat || rect.min.Long != c.minlong || rect.max.Lat != c.maxlat || rect.max.Long != c.maxlong { //wrong coordinates of rectangle
				t.Log("ERROR: got inconsistent coordinates")
				t.Fail()
			}
		}
	}
}

var testRectangles = []struct {
	r              *Rectangle
	expectedArea   float64
	expectedMargin float64
	expectedCenter Point
}{
	{&Rectangle{Point{0, 0}, Point{0, 0}}, 0, 0, Point{0, 0}},
	{&Rectangle{Point{1, 1}, Point{0, 0}}, 1, 4, Point{0.5, 0.5}},
	{&Rectangle{Point{0, 0}, Point{-1, -1}}, 1, 4, Point{-0.5, -0.5}},
	{&Rectangle{Point{10, 0}, Point{0, 0}}, 0, 20, Point{5, 0}},
	{&Rectangle{Point{10, 10}, Point{0, 0}}, 100, 40, Point{5, 5}},
}

func TestArea(t *testing.T) {
	for _, c := range testRectangles {
		res := c.r.Area()
		if res != c.expectedArea {
			t.Log("ERROR: got", res, "want", c.expectedArea)
			t.Fail()
		}
	}
}

func TestMargin(t *testing.T) {
	for _, c := range testRectangles {
		res := c.r.Margin()
		if res != c.expectedMargin {
			t.Log("ERROR: got", res, "want", c.expectedMargin)
			t.Fail()
		}
	}
}

func TestCenter(t *testing.T) {
	for _, c := range testRectangles {
		res := c.r.Center()
		if res != c.expectedCenter {
			t.Log("ERROR: got", res, "want", c.expectedCenter)
			t.Fail()
		}
	}
}

func TestContainsPoint(t *testing.T) {
	rect := Rectangle{Point{10, 10}, Point{-10, -10}}
	cases := []struct {
		p        Point
		expected bool
	}{
		{Point{0, 0}, true},
		{Point{10, 10}, true},
		{Point{-10, -10}, true},
		{Point{10, -10}, true},
		{Point{-10, 10}, true},
		{Point{10.000001, 0}, false},
		{Point{10, 10.000001}, false},
		{Point{900000, 900000}, false},
	}
	for _, c := range cases {
		res := rect.ContainsPoint(c.p)
		if res != c.expected {
			t.Log("ERROR: expected", c.expected, "got", res, " from case", c)
			t.Fail()
		}
	}
}

var testRectanglePairs = []struct {
	r                         *Rectangle
	other                     *Rectangle
	expectedContainsRectangle bool
	expectedOverlaps          bool
	expectedOverlapWith       float64
	expectedAreaDifference    float64
}{
	{&Rectangle{Point{0, 0}, Point{0, 0}}, &Rectangle{Point{0, 0}, Point{0, 0}}, true, true, 0, 0},                   //two "0-rectangles"
	{&Rectangle{Point{5, 5}, Point{-5, -5}}, &Rectangle{Point{20, 5}, Point{10, -5}}, false, false, 0, 0},            //two disjoint rectangles, same size
	{&Rectangle{Point{20, 5}, Point{10, -5}}, &Rectangle{Point{5, 5}, Point{-5, -5}}, false, false, 0, 0},            //same as above, different order
	{&Rectangle{Point{1, 1}, Point{0, 0}}, &Rectangle{Point{2, 1}, Point{1, 0}}, false, true, 0, 0},                  //two rectangles next to eachother, same size
	{&Rectangle{Point{1, 5}, Point{0, 0}}, &Rectangle{Point{2, 3}, Point{-1, 2}}, false, true, 1, 2},                 //two rectangles overlapping in a "cross", unequal size
	{&Rectangle{Point{0, 0}, Point{-2, -2}}, &Rectangle{Point{1, 1}, Point{-1, -1}}, false, true, 1, 0},              //two rectangles overlapping, same size
	{&Rectangle{Point{50, 50}, Point{0, 0}}, &Rectangle{Point{50, 50}, Point{0, 0}}, true, true, 2500, 0},            //two equal rectangles
	{&Rectangle{Point{0, 0}, Point{-50, -50}}, &Rectangle{Point{-20, -20}, Point{-30, -30}}, true, true, 100, 2400},  //one rectangle within the other rectangle
	{&Rectangle{Point{-20, -20}, Point{-30, -30}}, &Rectangle{Point{0, 0}, Point{-50, -50}}, false, true, 100, 2400}, //same as above, different order
	{&Rectangle{Point{1, 1}, Point{0, 0}}, &Rectangle{Point{1, 3}, Point{0, 2}}, false, false, 0, 0},                 // one rectangle above the other
	{&Rectangle{Point{1, 3}, Point{0, 2}}, &Rectangle{Point{1, 1}, Point{0, 0}}, false, false, 0, 0},                 //same as above, different order
	{&Rectangle{Point{4, 4}, Point{0, 0}}, &Rectangle{Point{5, 3}, Point{3, 1}}, false, true, 2, 12},                 //two overlapping rectangles
	{&Rectangle{Point{5, 3}, Point{3, 1}}, &Rectangle{Point{4, 4}, Point{0, 0}}, false, true, 2, 12},                 //same as above, different order
}

func TestContainsRectangle(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := c.r.ContainsRectangle(c.other)
		if res != c.expectedContainsRectangle {
			t.Log("ERROR: Got", res, "want", c.expectedContainsRectangle, " Rectangles ", c.r, c.other)
			t.Fail()
		}
	}
}

func TestOverlaps(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := Overlaps(c.r, c.other)
		if res != c.expectedOverlaps {
			t.Log("ERROR: Got", res, "want", c.expectedOverlaps, " c: ", c)
			t.Fail()
		}
	}
}

func TestOverlapWith(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := c.r.OverlapWith(c.other)
		if res != c.expectedOverlapWith {
			t.Log("ERROR: Got", res, "want", c.expectedOverlapWith, " c: ", c)
			t.Fail()
		}
	}
}

func TestAreaDifference(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := c.r.AreaDifference(c.other)
		if res != c.expectedAreaDifference {
			t.Log("ERROR: Got", res, "want", c.expectedAreaDifference)
			t.Fail()
		}
	}
}

func TestMBRWith(t *testing.T) {
	cases := []struct {
		r           *Rectangle
		other       *Rectangle
		expectedMBR *Rectangle
	}{
		{&Rectangle{Point{1, 1}, Point{0, 0}}, &Rectangle{Point{2, 1}, Point{1, 0}}, &Rectangle{Point{2, 1}, Point{0, 0}}},
		{&Rectangle{Point{0, 0}, Point{0, 0}}, &Rectangle{Point{0, 0}, Point{0, 0}}, &Rectangle{Point{0, 0}, Point{0, 0}}},
		{&Rectangle{Point{0, 0}, Point{-50, -50}}, &Rectangle{Point{0, 0}, Point{-20, -20}}, &Rectangle{Point{0, 0}, Point{-50, -50}}},
		{&Rectangle{Point{1, 1}, Point{0, 0}}, &Rectangle{Point{50, 1}, Point{40, 1}}, &Rectangle{Point{50, 1}, Point{0, 0}}},
	}
	for _, c := range cases {
		rect := c.r.MBRWith(c.other)
		if rect == nil {
			t.Log("ERROR: returned nil")
			t.Fail()
		} else {
			if rect.min.Lat != c.expectedMBR.min.Lat || rect.min.Long != c.expectedMBR.min.Long || rect.max.Lat != c.expectedMBR.max.Lat || rect.max.Long != c.expectedMBR.max.Long {
				t.Log("ERROR: wrong MBR")
				t.Fail()
			}
			rect2 := c.other.MBRWith(c.r)
			if rect2.min.Lat != c.expectedMBR.min.Lat || rect2.min.Long != c.expectedMBR.min.Long || rect2.max.Lat != c.expectedMBR.max.Lat || rect2.max.Long != c.expectedMBR.max.Long {
				t.Log("ERROR: wrong MBR2")
				t.Fail()
			}
		}
	}
}