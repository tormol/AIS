//go test -v -race || go test -v || go test -bench '.'
package storage

import (
	"encoding/json"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type posMessage struct {
	mmsi    uint32
	long    float64
	lat     float64
	heading uint16
}

func randShips(nShips, nMessages int) *map[uint32][]posMessage {
	m := make(map[uint32][]posMessage, nShips)
	for i := uint32(0); int(i) < nShips; i++ {
		m[i] = make([]posMessage, nMessages)
		for j := 0; j < nMessages; j++ {
			m[i] = append(m[i], randPos(i))
		}
	}
	return &m
}

//returns a random posMessage for the ship
func randPos(mmsi uint32) posMessage {
	long := float64(rand.Int31n(180)) * RandSign()
	lat := float64(rand.Int31n(90)) * RandSign()
	heading := uint16(rand.Int31n(360))
	return posMessage{mmsi, long, lat, heading}
}

func new(n, m int) (*ShipInfo, *map[uint32][]posMessage) {
	si := NewShipInfo()
	ships := randShips(n, m)
	for _, s := range *ships {
		for _, m := range s {
			si.AddCheckpoint(m.mmsi, m.lat, m.long, time.Now(), m.heading)
		}
	}
	return si, ships
}

/*TESTS*/
//Check for errors and concurrency
func TestAddCheckpoint(t *testing.T) {
	si := NewShipInfo()
	var wg sync.WaitGroup
	nShips := 500
	nMessages := 200
	ships := randShips(nShips, nMessages)
	wg.Add(nShips)
	for _, s := range *ships {
		go func(s []posMessage) {
			defer wg.Done()
			for _, m := range s {
				err := si.AddCheckpoint(m.mmsi, m.lat, m.long, time.Now(), m.heading)
				if err != nil {
					t.Log("ERROR: ", err)
					t.Fail()
				}
			}
		}(s)
	}
	wg.Wait()
	//Check if all ships got added
	for i := 0; i < nShips; i++ {
		if !si.Known(uint32(i)) {
			t.Log("ERROR: mmsi", i, "is not known, but should be")
			t.Fail()
		}
	}
}

func TestGetCoords(t *testing.T) {
	numShips := 1000
	m := 100
	si, ships := new(numShips, m)
	var wg sync.WaitGroup
	wg.Add(numShips)
	for _, s := range *ships {
		go func(s []posMessage) {
			defer wg.Done()
			lat, long := si.Coords(s[m-1].mmsi)
			if s[m-1].lat != lat || s[m-1].long != long {
				t.Log("ERROR: expected", s[m-1].lat, s[m-1].long, "got", lat, long)
				t.Fail()
			}
		}(s)
	}
	wg.Wait()
}

func TestUpdateSVD(t *testing.T) {
	si := NewShipInfo()
	n := 1000 //number of ships
	m := 100  //number of updates per ship
	var wg sync.WaitGroup
	wg.Add(n - 1)
	for i := 1; i < n; i++ {
		go func(mmsi uint32) {
			defer wg.Done()
			for j := 0; j < m; j++ {
				err := si.UpdateSVD(mmsi, "CALLSIGN", "DEST", "NAME", 10, 10)
				if err != nil {
					t.Log("ERROR: ", err)
					t.Fail()
				}
			}
		}(uint32(i))
	}
	wg.Wait()
	if len(si.allInfo) != n-1 {
		t.Log("ERROR: expected", n-1, "ships, but found", len(si.allInfo))
		t.Fail()
	}

	//More SVD messages
	cases := []struct {
		mmsi          uint32
		callsign      string
		destination   string
		name          string
		toBow         uint16
		toStern       uint16
		expectedError bool
	}{
		{uint32(n), "CALL", "DEST", "NAME", 12, 12, true}, //mmsi must be bigger than 0
		{uint32(n + 1), "CALL", "DEST", "NAME", 0, 0, false},
		{uint32(n + 2), "CALL", "DEST", "NAME", 0, 0, false},
		{uint32(n + 3), "", "", "", 1, 2, false},
		{uint32(n + 2), "", "", "NEW_NAME", 0, 0, false},       //updating mmsi: 0
		{uint32(n + 1), "CALL", "NEW_DEST", "", 10, 10, false}, //updating mmsi: 1
	}
	for _, c := range cases {
		err := si.UpdateSVD(c.mmsi, c.callsign, c.destination, c.name, c.toBow, c.toStern)
		if err != nil && !c.expectedError {
			t.Log("ERROR: UpdateSVD returned an error:", err)
			t.Fail()
		}
	}
	//Testing if the ships updated correctly:
	if si.allInfo[uint32(n+2)].Name != "NEW_NAME" {
		t.Log("ERROR: Failed to update info... got", si.allInfo[0].Name)
		t.Fail()
	}
	if si.allInfo[uint32(n+1)].Length != 20 {
		t.Log("ERROR: Failed to update info... got", si.allInfo[1].Length)
		t.Fail()
	}
	//Adding checkpoints to the ships
	for _, c := range cases {
		if !si.Known(c.mmsi) && !c.expectedError {
			t.Log("ERROR:", c.mmsi, "is not known but should be")
			t.Fail()
		} else {
			m := randPos(c.mmsi)
			err := si.AddCheckpoint(m.mmsi, m.lat, m.long, time.Now(), m.heading)
			if err != nil {
				t.Log("ERROR: got error when adding checkpoint:", err)
				t.Fail()
			}
		}
	}
}

func TestProperties(t *testing.T) {
	cases := []struct {
		mmsi    uint32
		call    string
		dest    string
		heading uint16
		name    string
		length  uint16
	}{
		{0, "CALL0", "DEST0", 0, "NAME0", 0}, //heading and length is included in the json even though it is 0...
		{1, "CALL1", "", 1, "NAME1", 10},
		{2, "", "", 360, "", 20},
		{3, "", "", 90, "", 30},
	}
	for _, c := range cases {
		i := info{c.name, c.dest, c.heading, c.call, c.length, []checkpoint{}, 0, &sync.Mutex{}}
		p := i.properties("name", "destination", "heading", "callsign", "length")
		var b info
		err := json.Unmarshal(p, &b)
		if err != nil {
			t.Log("ERROR", err)
		}
		//Check if the values are correct
		if b.Callsign != c.call {
			t.Log("ERROR: got", b.Callsign, "expected", c.call)
			t.Fail()
		}
		if b.Destination != c.dest {
			t.Log("ERROR: got", b.Destination, "expected", c.dest)
			t.Fail()
		}
		if b.Heading != c.heading {
			t.Log("ERROR: got", b.Heading, "expected", c.heading)
			t.Fail()
		}
		if b.Length != c.length {
			t.Log("ERROR: got", b.Length, "expected", c.length)
			t.Fail()
		}
		if b.Name != c.name {
			t.Log("ERROR: got", b.Name, "expected", c.name)
			t.Fail()
		}
	}
}

/*BENCHMARKS*/
// Add n ships with 1 checkpoints
func BenchmarkAddCheckpoint_ships(b *testing.B) {
	ships := randShips(b.N, 1) //n ships with 1 checkpoint
	si := NewShipInfo()
	b.ResetTimer() //start the timer from here
	for _, s := range *ships {
		si.AddCheckpoint(s[0].mmsi, s[0].lat, s[0].long, time.Now(), s[0].heading)
	}
}

// Add n checkpoints to the same ship
func BenchmarkAddCheckpoint_checkpoints(b *testing.B) {
	ships := make([]posMessage, b.N)
	for i := 0; i < b.N; i++ {
		ships[i] = randPos(uint32(i))
	}
	si := NewShipInfo()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si.AddCheckpoint(ships[i].mmsi, ships[i].lat, ships[i].long, time.Now(), ships[i].heading)
	}
}

// Adding n ships
func BenchmarkUpdateSVD(b *testing.B) {
	si := NewShipInfo()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si.UpdateSVD(uint32(i), "CALL", "DEST", "NAME", 1, 1)
	}
}

func BenchmarkAllInfo(b *testing.B) {
	si, _ := new(b.N, 100) // n ships with 100 positions
	for i := 0; i < b.N; i++ {
		si.UpdateSVD(uint32(i), "CALL", "DEST", "NAME", 10, 10)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si.AllInfo(uint32(i))
	}
}

//References: https://golang.org/doc/articles/race_detector.html
