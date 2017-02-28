// a hashmap used to store information and history for each boat
package storage

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

const HISTORY_MAX = 100 // The maximum number of points allowed to be stored in the history
const HISTORY_MIN = 60  // The minimum number of points stored in the history

// "A point in history"
type checkpoint struct {
	lat  float64
	long float64
	t    time.Time
}

//TODO decide what information to store for each boat
type info struct {
	name        string // Ship's name
	destination string // Destination
	heading     uint16 // between 0 and 359 degrees	- can be used to rotate the leaflet marker
	callsign    string
	length      uint16       // Length of the ship (in meters)
	history     []checkpoint // first checkpoint is the oldest
	hLength     uint16       // current number of checkpoints in the history
	mu          *sync.Mutex  // Mutex lock for currency
}

// Contains an info-object for each ship
type ShipInfo struct {
	allInfo map[uint32]*info
	rw      *sync.RWMutex
}

func NewShipInfo() *ShipInfo {
	return &ShipInfo{make(map[uint32]*info), &sync.RWMutex{}}
}

// Is the ship known?
func (si *ShipInfo) IsKnown(mmsi uint32) bool {
	si.rw.RLock()
	_, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	return ok
}

// Adds a new checkpoint to the ship (this is called for every(?) AIS message)
func (si *ShipInfo) AddCheckpoint(mmsi uint32, nlat, nlong float64, nt time.Time, heading uint16) error {
	if !LegalCoord(nlat, nlong) || nt.IsZero() {
		return errors.New("Illegal checkpoint")
	}
	si.rw.RLock()
	i, ok := si.allInfo[mmsi] //Get the info-object for this ship
	si.rw.RUnlock()
	if !ok { // A new ship
		i = &info{history: make([]checkpoint, HISTORY_MAX), hLength: 0, mu: &sync.Mutex{}}
		si.rw.Lock()
		si.allInfo[mmsi] = i
		si.rw.Unlock()
	}
	// Update the ship's info-object
	i.mu.Lock()
	i.history[i.hLength] = checkpoint{lat: nlat, long: nlong, t: nt}
	i.hLength++
	if i.hLength >= HISTORY_MAX { //purge the slice
		newH := make([]checkpoint, HISTORY_MAX)
		copy(newH, i.history[:HISTORY_MIN])
		i.history = newH
		i.hLength = HISTORY_MIN
	}
	if heading >= 0 && heading <= 359 {
		i.heading = heading
	}
	i.heading = heading
	i.mu.Unlock()
	return nil
}

// Update the Static Voyage Data
func (si *ShipInfo) UpdateSVD(mmsi uint32, callsign, destination, name string, toBow, toStern uint16) error {
	if mmsi <= 0 {
		return errors.New("Illegal MMSI")
	}
	si.rw.RLock()
	i, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	if !ok { // A new ship
		return nil // Only care about ships that have a position... for now...
	}
	i.mu.Lock()
	if callsign != "" {
		i.callsign = callsign
	}
	if destination != "" {
		i.destination = destination
	}
	if name != "" {
		i.name = name
	}
	if toBow >= 0 {
		i.length = toBow + toStern
	}
	i.mu.Unlock()
	return nil
}

//get duration since last message
func (si *ShipInfo) GetDuration(mmsi uint32) (time.Duration, error) {
	si.rw.RLock()
	i, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	if ok {
		i.mu.Lock()
		defer i.mu.Unlock()
		if i.hLength > 0 {
			return time.Since(i.history[i.hLength-1].t), nil
		}
	}
	return 0, errors.New("Can't find log of that ship")
}

//print the history a GeoJSON LineString object
func (si *ShipInfo) GetHistory(mmsi uint32) string {
	si.rw.RLock()
	i, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	if !ok {
		return ""
	}
	k := uint16(0) // used in the for loop
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.hLength <= 1 { // A LineString must contain at least 2 points
		return ""
	}
	s := make([]string, 0, i.hLength)
	for k < i.hLength {
		s = append(s, "["+strconv.FormatFloat(i.history[k].long, 'f', 6, 64)+", "+strconv.FormatFloat(i.history[k].lat, 'f', 6, 64)+"]") //GeoJSON uses <long, lat> instead of <lat, long> ...
		k++
	}
	return `{
		"type": "LineString",
		"coordinates": [` +
		strings.Join(s, ", ") +
		`]
	}`
}

//get last position of ship
func (si *ShipInfo) GetCoords(mmsi uint32) (lat, long float64) {
	si.rw.RLock()
	i, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	if ok {
		i.mu.Lock()
		defer i.mu.Unlock()
		if i.hLength > 0 {
			lat = i.history[i.hLength-1].lat
			long = i.history[i.hLength-1].long
		}
	}
	return
}

//Get name, length, and heading of the ship (used as part of the GeoJSON Feature object for the ship)
func (si *ShipInfo) GetFeatures(mmsi uint32) (string, uint16, uint16) {
	si.rw.RLock()
	i, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	if ok {
		i.mu.Lock()
		defer i.mu.Unlock()
		return i.name, i.length, i.heading
	}
	return "", 0, 0
}

/*
References:
	https://en.wikipedia.org/wiki/Automatic_identification_system#Broadcast_information
	http://geojsonlint.com/
*/
