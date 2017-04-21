//go test -v -race || go test -v || go test -bench '.'
package storage

import (
	"encoding/json"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/tormol/AIS/geo"
)

func randShipsPos(nShips, nMessages int) *map[uint32][]ShipPos {
	m := make(map[uint32][]ShipPos)
	for i := 0; i < nShips; i++ {
		m[uint32(i)] = make([]ShipPos, 0, nMessages)
		for j := 0; j < nMessages; j++ {
			m[uint32(i)] = append(m[uint32(i)], randShipPos(j))
		}
	}
	return &m
}

//returns a random ShipPos for the ship
func randShipPos(extra int) ShipPos {
	long := float64(rand.Int31n(180)) * RandSign()
	lat := float64(rand.Int31n(90)) * RandSign()
	posAcc := Accuracy(true)
	navstat := ShipNavStatus(uint8(0))
	bowHeading := uint16(rand.Int31n(360))
	course := float32(rand.Int31n(360))
	speed := float32(rand.Int31n(80))
	rot := float32(rand.Int31n(360))
	return ShipPos{time.Now().Add(time.Duration(extra) * time.Nanosecond), geo.Point{lat, long}, posAcc, navstat, bowHeading, course, speed, rot}
}

func new(n, m int) (*ShipDB, *map[uint32][]ShipPos) {
	db := NewShipDB()
	ships := randShipsPos(n, m)
	for mmsi, s := range *ships {
		for _, m := range s {
			db.UpdateDynamic(mmsi, m)
		}
	}
	return db, ships
}

/*TESTS*/
//Check for errors and concurrency
func TestUpdateDynamic(t *testing.T) {
	db := NewShipDB()
	var wg sync.WaitGroup
	nShips := 100
	nMessages := 80
	ships := randShipsPos(nShips, nMessages)
	wg.Add(nShips)
	for mmsi, s := range *ships {
		go func(messages []ShipPos, mmsi uint32) {
			defer wg.Done()
			for _, m := range messages {
				db.UpdateDynamic(mmsi, m)
			}
		}(s, mmsi)
	}
	wg.Wait()
	//Check if all ships got added
	for i := uint32(0); int(i) < nShips; i++ {
		if !db.Known(i) {
			t.Log("ERROR: mmsi", i, "is not known, but should be")
			t.Fail()
		}
	}
}

func TestUpdateStatic(t *testing.T) {
	db := NewShipDB()
	n := 1500 //number of ships
	m := 300  //number of updates per ship
	var wg sync.WaitGroup
	wg.Add(n - 1)
	for i := 1; i < n; i++ {
		go func(mmsi uint32) {
			defer wg.Done()
			for j := 0; j < m; j++ {
				db.UpdateStatic(mmsi, ShipInfo{1, 1, 1, 1, 1, "CALL", "NAME", 1, "SOME_DEST", time.Now()})
			}
		}(uint32(i))
	}
	wg.Wait()
	//Check if all ships got added
	if len(db.ships) != n-1 {
		t.Log("ERROR: expected", n-1, "ships, but found", len(db.ships))
		t.Fail()
	}
	for i := 1; i < n; i++ {
		if !db.Known(uint32(i)) {
			t.Log("ERROR: mmsi", i, "is not known, but should be")
			t.Fail()
		}
	}
	//More SVD messages
	cases := []struct {
		mmsi    uint32
		message ShipInfo
	}{
		{uint32(n), ShipInfo{Length: 15}},
		{uint32(n + 1), ShipInfo{Length: 0, LengthOffset: 1}},
		{uint32(n + 2), ShipInfo{Callsign: "CALL", ShipName: "NAME"}},
		{uint32(n + 3), ShipInfo{}},
		{uint32(n + 2), ShipInfo{ShipName: "NEW_NAME"}},         //updating mmsi: n+2
		{uint32(n + 1), ShipInfo{Length: 20, Dest: "NEW_DEST"}}, //updating mmsi: n+1
	}
	for _, c := range cases {
		db.UpdateStatic(c.mmsi, c.message)
	}
	//Testing if the ships updated correctly:
	if db.ships[uint32(n+2)].ShipName != "NEW_NAME" {
		t.Log("ERROR: Failed to update info... got", db.ships[uint32(n+2)])
		t.Fail()
	}
	if db.ships[uint32(n+1)].Length != 20 {
		t.Log("ERROR: Failed to update info... got", db.ships[uint32(n+1)].Length)
		t.Fail()
	}
	//Adding checkpoints to the ships
	for _, c := range cases {
		if !db.Known(c.mmsi) {
			t.Log("ERROR:", c.mmsi, "is not known but should be")
			t.Fail()
		} else {
			m := randShipPos(1)
			db.UpdateDynamic(c.mmsi, m)
		}
	}
}

func TestGeoJSON(t *testing.T) {
	cases := []struct {
		mmsi    uint32
		call    string
		dest    string
		heading uint16
		name    string
		length  uint16
	}{
		{4, "CALL0", "DEST0", 0, "NAME0", 0},
		{1, "CALL1", "", 1, "NAME1", 10},
		{2, "", "", 360, "", 20},
		{3, "", "", 90, "", 30},
	}
	for _, c := range cases {
		i := ship{Mmsi(c.mmsi).Owner(), Mmsi(c.mmsi).CountryCode(), ShipInfo{Length: c.length, Dest: c.dest, Callsign: c.call, ShipName: c.name}, ShipPos{BowHeading: c.heading}, []checkpoint{}, 0, &sync.Mutex{}}
		p, err := json.Marshal(i)
		if err != nil {
			t.Log("ERROR", err)
			t.Fail()
		}
		var b ship
		err = json.Unmarshal(p, &b)
		if err != nil {
			t.Log("ERROR, could not unmarshal the ship object:", string(p), "... got error: ", err)
			t.Fail()
		}
		//Check if the values are correct
		if b.Callsign != c.call {
			t.Log("ERROR: got", b.Callsign, "expected", c.call)
			t.Fail()
		}
		if b.Dest != c.dest {
			t.Log("ERROR: got", b.Dest, "expected", c.dest)
			t.Fail()
		}
		if b.BowHeading != c.heading {
			t.Log("ERROR: got", b.BowHeading, "expected", c.heading)
			t.Fail()
		}
		if b.Length != c.length {
			t.Log("ERROR: got", b.Length, "expected", c.length)
			t.Fail()
		}
		if b.ShipName != c.name {
			t.Log("ERROR: got", b.ShipName, "expected", c.name)
			t.Fail()
		}
	}
}

func TestCoords(t *testing.T) {
	n := 500
	m := 123
	db, ships := new(n, m)
	var wg sync.WaitGroup
	wg.Add(n)
	for mmsi, s := range *ships {
		go func(s []ShipPos, mmsi uint32) {
			defer wg.Done()
			lat, long := db.Coords(mmsi)
			if s[m-1].Pos.Lat != lat || s[m-1].Pos.Long != long {
				t.Log("ERROR: expected", s[m-1].Pos.Lat, s[m-1].Pos.Long, "got", lat, long, "for mmsi: ", mmsi)
				t.Fail()
			}
		}(s, mmsi)
	}
	wg.Wait()
}

/*BENCHMARKS*/
// Add n ships with 1 checkpoints
func BenchmarkUpdateDynamic_ships(b *testing.B) {
	ships := randShipsPos(b.N, 1) //n ships with 1 checkpoint
	db := NewShipDB()
	b.ResetTimer() //start the timer from here
	for mmsi, s := range *ships {
		db.UpdateDynamic(mmsi, s[0])
	}
}

// Add n checkpoints to the same ship
func BenchmarkUpdateDynamic_checkpoints(b *testing.B) {
	ships := make([]ShipPos, b.N)
	for i := 0; i < b.N; i++ {
		ships[i] = randShipPos(i)
	}
	db := NewShipDB()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.UpdateDynamic(uint32(i), ships[i])
	}
}

// Adding n ships
func BenchmarkUpdateStatic(b *testing.B) {
	db := NewShipDB()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.UpdateStatic(uint32(i), ShipInfo{1, 1, 1, 1, 1, "CALL", "NAME", 1, "SOME_DEST", time.Now()})
	}
}

func BenchmarkSelect(b *testing.B) {
	db, _ := new(b.N, 100) // n ships with 100 positions
	for i := 0; i < b.N; i++ {
		db.UpdateDynamic(uint32(i), ShipPos{time.Now(), geo.Point{1, 1}, false, 0, 0, 0, 0, 0})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.Select(uint32(i))
	}
}

//References: https://golang.org/doc/articles/race_detector.html
