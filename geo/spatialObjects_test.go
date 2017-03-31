package geo

import (
	"encoding/json"
	"fmt"
	"math"
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

func TestMarshalJSON(t *testing.T) {
	cases := []struct {
		p        Point
		expected string
	}{
		{Point{0, 0}, `[0,0]`},
		{Point{80.706050, -170.809010}, `[-170.80901,80.70605]`},
		{Point{0.100000, -0.100000}, `[-0.1,0.1]`},
	}
	for _, c := range cases {
		j, err := json.Marshal(c.p)
		if err != nil {
			t.Log("ERROR:", err)
			t.Fail()
		}
		if string(j) != c.expected {
			t.Log("ERROR: expected:\n", c.expected, "\ngot:\n", string(j))
			t.Fail()
		}
	}
}

func TestUnmarshalJSON(t *testing.T) {
	cases := []struct {
		json     []byte
		expected Point
	}{
		{[]byte{'[', '1', '.', '2', '3', ',', '2', '.', '3', ']'}, Point{2.3, 1.23}},
		{[]byte{'[', '0', ',', '0', ']'}, Point{0, 0}},
	}
	for _, c := range cases {
		var got Point
		err := json.Unmarshal(c.json, &got)
		if err != nil {
			t.Log("ERROR:", err)
			t.Fail()
		}
		if got != c.expected {
			t.Log("ERROR:got", got, "expected", c.expected)
			t.Fail()
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
	r                         Rectangle
	other                     Rectangle
	expectedContainsRectangle bool
	expectedOverlaps          bool
	expectedOverlapWith       float64
	expectedAreaDifference    float64
}{
	{Rectangle{Point{0, 0}, Point{0, 0}}, Rectangle{Point{0, 0}, Point{0, 0}}, true, true, 0, 0},                   //two "0-rectangles"
	{Rectangle{Point{5, 5}, Point{-5, -5}}, Rectangle{Point{20, 5}, Point{10, -5}}, false, false, 0, 0},            //two disjoint rectangles, same size
	{Rectangle{Point{20, 5}, Point{10, -5}}, Rectangle{Point{5, 5}, Point{-5, -5}}, false, false, 0, 0},            //same as above, different order
	{Rectangle{Point{1, 1}, Point{0, 0}}, Rectangle{Point{2, 1}, Point{1, 0}}, false, true, 0, 0},                  //two rectangles next to eachother, same size
	{Rectangle{Point{1, 5}, Point{0, 0}}, Rectangle{Point{2, 3}, Point{-1, 2}}, false, true, 1, 2},                 //two rectangles overlapping in a "cross", unequal size
	{Rectangle{Point{0, 0}, Point{-2, -2}}, Rectangle{Point{1, 1}, Point{-1, -1}}, false, true, 1, 0},              //two rectangles overlapping, same size
	{Rectangle{Point{50, 50}, Point{0, 0}}, Rectangle{Point{50, 50}, Point{0, 0}}, true, true, 2500, 0},            //two equal rectangles
	{Rectangle{Point{0, 0}, Point{-50, -50}}, Rectangle{Point{-20, -20}, Point{-30, -30}}, true, true, 100, 2400},  //one rectangle within the other rectangle
	{Rectangle{Point{-20, -20}, Point{-30, -30}}, Rectangle{Point{0, 0}, Point{-50, -50}}, false, true, 100, 2400}, //same as above, different order
	{Rectangle{Point{1, 1}, Point{0, 0}}, Rectangle{Point{1, 3}, Point{0, 2}}, false, false, 0, 0},                 // one rectangle above the other
	{Rectangle{Point{1, 3}, Point{0, 2}}, Rectangle{Point{1, 1}, Point{0, 0}}, false, false, 0, 0},                 //same as above, different order
	{Rectangle{Point{4, 4}, Point{0, 0}}, Rectangle{Point{5, 3}, Point{3, 1}}, false, true, 2, 12},                 //two overlapping rectangles
	{Rectangle{Point{5, 3}, Point{3, 1}}, Rectangle{Point{4, 4}, Point{0, 0}}, false, true, 2, 12},                 //same as above, different order
}

func TestContainsRectangle(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := c.r.ContainsRectangle(&c.other)
		if res != c.expectedContainsRectangle {
			t.Log("ERROR: Got", res, "want", c.expectedContainsRectangle, " Rectangles ", c.r, c.other)
			t.Fail()
		}
	}
}

func TestOverlaps(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := Overlaps(&c.r, &c.other)
		if res != c.expectedOverlaps {
			t.Log("ERROR: Got", res, "want", c.expectedOverlaps, " c: ", c)
			t.Fail()
		}
	}
}

func TestOverlapWith(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := c.r.OverlapWith(&c.other)
		if res != c.expectedOverlapWith {
			t.Log("ERROR: Got", res, "want", c.expectedOverlapWith, " c: ", c)
			t.Fail()
		}
	}
}

func TestAreaDifference(t *testing.T) {
	for _, c := range testRectanglePairs {
		res := c.r.AreaDifference(&c.other)
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

// shorter than NewRectangle and doesn't return an error
func r(minLat, minLong, maxLat, maxLong float64) Rectangle {
	return Rectangle{
		min: Point{minLat, minLong},
		max: Point{maxLat, maxLong},
	}
}

var splitViewRectTests = []struct {
	input Rectangle
	want  []Rectangle
}{
	{r(0, 0, 0, 0), []Rectangle{r(0, 0, 0, 0)}},
	{r(-90, -180, 90, 180), []Rectangle{r(-90, -180, 90, 180)}},
	{r(0, -179, 0, 180), []Rectangle{r(0, -179, 0, 180)}},
	{r(0, -179, 0, 181), []Rectangle{r(0, -180, 0, 180)}},
	{r(0, -179, 0, 182), []Rectangle{r(0, -180, 0, 180)}},
	{r(0, 110, 0, 180), []Rectangle{r(0, 110, 0, 180)}},
	{r(0, 110, 0, 181), []Rectangle{r(0, -180, 0, -179), r(0, 110, 0, 180)}},
	{r(0, 110, 0, 10), []Rectangle{r(0, -180, 0, 10), r(0, 110, 0, 180)}},
	{r(85, 10, 95, 20), nil}, // TODO: {r(85, 10, 95, 20), []Rectangle{r(85, -170, 90, -160), r(85, 10, 90, 20)}},
	{r(1, 0, -1, 0), nil},
}

func TestSplitViewRect(t *testing.T) {
	fail := func(in Rectangle, want, got []Rectangle) {
		print := func(r Rectangle) string {
			return fmt.Sprintf("{(%.0f,%.0f), (%.0f,%.0f)}",
				r.Min().Lat, r.Min().Long, r.Max().Lat, r.Max().Long,
			)
		}
		msg := print(in) + "\nwant:"
		for _, r := range want {
			msg += "\n\t" + print(r)
		}
		msg += "\ngot:"
		for _, r := range got {
			msg += "\n\t" + print(r)
		}
		t.Error(msg)
	}
	test := func(input Rectangle, want []Rectangle) {
		got := SplitViewRect(input.Min().Lat, input.Min().Long, input.Max().Lat, input.Max().Long)
		if len(got) != len(want) {
			fail(input, want, got)
		} else {
			for i := range got {
				if got[i] != want[i] {
					fail(input, want, got)
					break
				}
			}
		}
	}
	for _, c := range splitViewRectTests {
		test(c.input, c.want)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(-1), math.Inf(1)} {
		test(r(bad, 0, 0, 0), nil)
		test(r(0, bad, 0, 0), nil)
		test(r(0, 0, bad, 0), nil)
		test(r(0, 0, 0, bad), nil)
	}
}
