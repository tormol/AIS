package storage

import (
	"math"
	"math/rand"
	"testing"

	"github.com/tormol/AIS/geo"
)

var mmsiCount uint32

type testBoat struct {
	mmsi uint32
	long float64
	lat  float64
}

//Create n boats with random coordinates
func createBoats(n int) []testBoat {
	mmsiCount = 0 //reset the mmsi
	boats := make([]testBoat, n, n)
	for i := 0; i < n; i++ {
		b := randBoat()
		boats[i] = b
	}
	return boats
}

func randBoat() testBoat {
	long := float64(rand.Int31n(180)) * RandSign()
	lat := float64(rand.Int31n(90)) * RandSign()
	mmsi := mmsiCount
	mmsiCount++
	return testBoat{mmsi, long, lat}
}

func TestCondenseSingleLeaf(t *testing.T) {
	rt := NewRTree()
	// insert two entries
	rt.InsertData(0.0, 0.0, 0)
	rt.InsertData(1.0, 1.0, 1)
	// when one is removed, there is a single node left in root, which root can replace root..
	// except it's a leaf node, (= with a nil node pointer) because then root would become nil.
	// (Update removes and inserts, if it root was set to nil inset will dereference it)
	rt.Update(1, 1.0, 1.0, -1.0, -1.0)
}

func TestInsertData(t *testing.T) {
	num := 100000
	rt := NewRTree()
	boats := createBoats(num)
	var err error
	for i := 0; i < len(boats); i++ {
		err = rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)
		if err != nil { // shouldn't get error, but got
			t.Log("ERROR: should be <nil>, but got ", err) //print message to screen
			t.Fail()                                       //indicates that the test failed
		}
	}
	//Test if the correct amount of ships is found
	if num != rt.NumOfBoats() { //Testing the NumOfBoats func
		t.Log("ERROR: wrong number of boats. Expected", num, "got", rt.NumOfBoats())
		t.Fail()
	}

	failedBoats := []testBoat{
		{mmsiCount + 1, 1, 91},
		{mmsiCount + 2, 1, -91},
		{mmsiCount + 3, 181, 1},
		{mmsiCount + 4, -181, 1},
	}
	for _, b := range failedBoats {
		err = rt.InsertData(b.lat, b.long, b.mmsi)
		if err == nil {
			t.Log("ERROR: insert should fail but did not")
			t.Fail()
		}
	}
	all, _ := geo.NewRectangle(-90, -180, 90, 180)
	numFound := len(*rt.FindWithin(all))
	if num != numFound {
		t.Log("FindAll did not find the correct amount of boats. Found", numFound, ", expected", num)
		t.Fail()
	}

}

func TestUpdate(t *testing.T) {
	numberOfBoats := 0
	rt := NewRTree()
	num := 100000
	boats := createBoats(num)
	var err error
	//Insert the boats
	for i := 0; i < num; i++ {
		err = rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)
		if err != nil { // shouldn't get error, but got
			t.Log("ERROR: should be <nil>, but got ", err, "... (during first insert)") //print message to screen
			t.Fail()                                                                    //indicates that the test failed
		}
		numberOfBoats++
	}
	if numberOfBoats != rt.NumOfBoats() { //Testing the NumOfBoats func
		t.Log("ERROR: wrong number of boats. Expected", numberOfBoats, "got", rt.NumOfBoats())
		t.Fail()
	}
	all, _ := geo.NewRectangle(-90, -180, 90, 180)
	numFound := len(*rt.FindWithin(all))
	if numberOfBoats != numFound {
		t.Log("FindAll did not find the correct amount of boats. Found", numFound, ", expected", numberOfBoats)
		t.Fail()
	}
	newBoats := createBoats(num) //new boats with new coordinates
	//Update the boats
	for i := 0; i < num; i++ {
		oldB := boats[i]
		newB := newBoats[i]
		err = rt.Update(uint32(i), oldB.lat, oldB.long, newB.lat, newB.long)
		if err != nil {
			t.Log("ERROR: error should be <nil>, but got", err, "... (while updating)")
			t.Log("oldB:", oldB, ", newB:", newB)
			t.Fail()
		}
	}
	//Checking the number of boats
	if numberOfBoats != rt.NumOfBoats() { //Testing the NumOfBoats func after updating the coordinates
		t.Log("ERROR: wrong number of boats. Expected", numberOfBoats, "got", rt.NumOfBoats())
		t.Fail()
	}
	numFound = len(*rt.FindWithin(all))
	if numberOfBoats != numFound {
		t.Log("FindAll did not find the correct amount of boats. Found", numFound, ", expected", numberOfBoats)
		t.Log(rt.FindWithin(all))
		t.Fail()
	}
}

func TestWithin(t *testing.T) {
	//Inserting the points
	rt := NewRTree()
	boats := []testBoat{ //mmsi, long, lat
		{0, 0, 0},
		{1, 10, 10},
		{2, -10, 10},
		{3, 10, -10},
		{4, -10, -10},
		{5, 2, 2},
		{6, 2, 2},
		{7, 50, 0},
		{8, 0, 50},
		{9, 5, 5},
		{10, -5, -5},
		{11, 5, -5},
		{12, -5, -5},
	}
	var err error
	for _, b := range boats {
		err = rt.InsertData(b.lat, b.long, b.mmsi)
		if err != nil { // shouldn't get error, but got
			t.Log("ERROR: should be <nil>, but got ", err, "... (during insert)") //print message to screen
			t.Fail()                                                              //indicates that the test failed
		}
	}
	//Creating the rectangles to search
	testRects := []struct {
		maxLat       float64 //the rectangle to search
		maxLong      float64
		minLat       float64
		minLong      float64
		expectedMMSI []uint32 //the expected matches within the rectangle
	}{
		{10, 10, -10, -10, []uint32{0, 1, 2, 3, 4, 5, 6, 9, 10, 11, 12}},
		{50, 50, -50, -50, []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{10, 10, 0, 0, []uint32{0, 1, 5, 6, 9}},
		{1, 180, -1, 20, []uint32{7}},
		{0, 0, 0, 0, []uint32{0}},
		{80, 80, 80, 80, []uint32{}},
	}
	for _, tr := range testRects {
		r, _ := geo.NewRectangle(tr.minLat, tr.minLong, tr.maxLat, tr.maxLong)
		matches := rt.FindWithin(r)
		if len(*matches) != len(tr.expectedMMSI) {
			t.Log("ERROR: incorrect number of matches, want", len(tr.expectedMMSI), "got", len(*matches), "within the rectangle", *r)
			t.Fail()
		} else {
			for _, m := range *matches {
				wasExpected := false
				for _, e := range tr.expectedMMSI {
					if m.MMSI == e {
						wasExpected = true
					}
				}
				if !wasExpected {
					t.Log("One of the matches found was not expected... found", m.MMSI, "within the testRect", tr)
					t.Fail()
				}
			}
		}
	}
}

/*	BENCHMARKS	*/
func BenchmarkInsertData(b *testing.B) {
	rt := NewRTree()
	boats := createBoats(b.N)
	b.ResetTimer() //start the timer from here
	for i := 0; i < b.N; i++ {
		rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)

	}
}

func BenchmarkUpdate(b *testing.B) {
	rt := NewRTree()
	boats := createBoats(b.N)
	for i := 0; i < b.N; i++ {
		rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)
	}
	newBoats := createBoats(b.N)
	b.ResetTimer() //start the timer from here
	for i := 0; i < b.N; i++ {
		rt.Update(uint32(i), boats[i].lat, boats[i].long, newBoats[i].lat, newBoats[i].long)
	}
}

//Searching of random rectangles (random size and position)
func BenchmarkFindWithin(b *testing.B) {
	rt := NewRTree()
	boats := createBoats(25000)
	for i := 0; i < 25000; i++ {
		rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)
	}
	rects := createRects(b.N)
	b.ResetTimer() //start the timer from here
	for i := 0; i < b.N; i++ {
		rt.FindWithin(rects[i])
	}
}

//the search uses random 18x18 rectangles (same size, random position)
func BenchmarkFindWithin18x18(b *testing.B) {
	rt := NewRTree()
	boats := createBoats(25000)
	for i := 0; i < 25000; i++ {
		rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)
	}
	rects := createFixedSizeRects(b.N)
	b.ResetTimer() //start the timer from here
	for i := 0; i < b.N; i++ {
		rt.FindWithin(rects[i])
	}
}

func BenchmarkFindAll(b *testing.B) {
	rt := NewRTree()
	boats := createBoats(25000)
	for i := 0; i < b.N; i++ {
		rt.InsertData(boats[i].lat, boats[i].long, boats[i].mmsi)
	}
	all, _ := geo.NewRectangle(-90, -180, 90, 180)
	b.ResetTimer() //start the timer from here
	for i := 0; i < b.N; i++ {
		rt.FindWithin(all)
	}
}

// Creates n random rectangles
func createRects(n int) []*geo.Rectangle {
	rects := make([]*geo.Rectangle, n, n)
	for i := 0; i < n; i++ {
		r := randRect()
		rects[i] = r
	}
	return rects
}

func randRect() *geo.Rectangle {
	long1 := float64(rand.Int31n(180)) * RandSign()
	lat1 := float64(rand.Int31n(90)) * RandSign()
	long2 := float64(rand.Int31n(180)) * RandSign()
	lat2 := float64(rand.Int31n(90)) * RandSign()
	r, _ := geo.NewRectangle(math.Min(lat1, lat2), math.Min(long1, long2), math.Max(lat1, lat2), math.Max(long1, long2))
	return r
}

func createFixedSizeRects(n int) []*geo.Rectangle {
	rects := make([]*geo.Rectangle, n, n)
	for i := 0; i < n; i++ {
		r := randFixedRect()
		rects[i] = r
	}
	return rects
}

//Uses 18lat * 18long rectangles
func randFixedRect() *geo.Rectangle {
	long1 := float64(rand.Int31n(342)) - 180
	long2 := float64(long1 + 18)
	lat1 := float64(rand.Int31n(162)) - 90
	lat2 := float64(lat1 + 18)
	r, _ := geo.NewRectangle(lat1, long1, lat2, long2)
	return r
}

//positive or negative
func RandSign() float64 {
	if rand.Intn(2) == 0 {
		return float64(-1)
	}
	return float64(1)
}
