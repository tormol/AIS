// a hashmap used to store information and history for each boat
package storage

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/tormol/AIS/geo"
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
	Name        string       `json:"name,omitempty"` //Omitted by json if empty
	Destination string       `json:"destination,omitempty"`
	Heading     uint16       `json:"heading,omitempty"` // between 0 and 359 degrees	- can be used to rotate the leaflet marker
	Callsign    string       `json:"callsign,omitempty"`
	Length      uint16       `json:"length,omitempty"`
	history     []checkpoint // first checkpoint is the oldest				(unexported fields is ignored by json)
	hLength     uint16       // current number of checkpoints in the history(ignored by json)
	mu          *sync.Mutex  // Mutex lock for currency						(ignored by json)
}

/* properties returns the ships geojson "properties" with the specified fields.
Example: properties("name", "heading", "length") 	returns: {"name": "SHIPNAME", "heading":123, "length":321}
Empty fields are omitted
*/
func (i *info) properties(fields ...string) []byte {
	tmpSet := make(map[string]bool, len(fields)) //A map of the specified fields
	for _, p := range fields {
		tmpSet[p] = true
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	t := reflect.TypeOf(*i)
	v := reflect.ValueOf(*i)
	prop := make(map[string]interface{}, t.NumField()) //The set containing the specified fields and its values
	for k := 0; k < t.NumField(); k++ {
		field := t.Field(k) //Gets the k'th field of t
		jsonKey := field.Tag.Get("json")
		//remove the omitempty part of the json tag
		//	- otherwise the geojson objects would be named like: "name,omitempty":"THESHIPNAME", "length,omitempty": 123, ...
		if len(jsonKey) > 11 && jsonKey[len(jsonKey)-9:] == "omitempty" {
			jsonKey = jsonKey[:len(jsonKey)-10] // if "name,omitempty"
		}
		if tmpSet[jsonKey] {
			value := v.Field(k).Interface()
			//manual omitempty
			if value == "" || value == nil || value == 0 {
				continue
			}
			prop[jsonKey] = value
		}
	}
	//Make the geojson object
	b, err := json.MarshalIndent(prop, "", "\t")
	if err != nil {
		panic(err.Error()) //FIXME
	}
	return b //Return the geoJSON string
}

// Contains an info-object for each ship
type ShipInfo struct {
	allInfo map[uint32]*info
	rw      *sync.RWMutex
}

// NewShipInfo creates and returns a pointer to a new ShipInfo object.
func NewShipInfo() *ShipInfo {
	return &ShipInfo{make(map[uint32]*info), &sync.RWMutex{}}
}

// IsKnown returns true if the given mmsi is stored in the structure.
func (si *ShipInfo) IsKnown(mmsi uint32) bool {
	si.rw.RLock()
	_, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	return ok
}

// Adds a new checkpoint to the ship (this is called for every(?) AIS message)
func (si *ShipInfo) AddCheckpoint(mmsi uint32, nlat, nlong float64, nt time.Time, heading uint16) error {
	if !geo.LegalCoord(nlat, nlong) || nt.IsZero() {
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
		i.Heading = heading
	}
	i.Heading = heading
	i.mu.Unlock()
	return nil
}

// UpdateSVD updates the Static Voyage Data for the ship.
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
		i.Callsign = callsign
	}
	if destination != "" {
		i.Destination = destination
	}
	if name != "" {
		i.Name = name
	}
	if toBow >= 0 {
		i.Length = toBow + toStern
	}
	i.mu.Unlock()
	return nil
}

// GetDuration returns the duration since last message recieved from the ship.
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

// GetCoords returns the last known position of ship.
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

/*GeoJSON structures*/
type feature struct {
	Type       string           `json:"type"`
	ID         uint32           `json:"id"`
	Geometry   geometry         `json:"geometry"`
	Properties *json.RawMessage `json:"properties"`
}

type geometry struct {
	Type        string      `json:"type"`
	Coordinates interface{} `json:"coordinates"`
}

var emptyJsonObject = json.RawMessage(`{}`) //empty struct

// GetAllInfo returns the info about the ship and its tracklog, in a geojson FeatureCollection.
func (si *ShipInfo) GetAllInfo(mmsi uint32) string {
	si.rw.RLock()
	i, ok := si.allInfo[mmsi]
	si.rw.RUnlock()
	if ok {
		properties := json.RawMessage(i.properties("name", "destination", "heading", "callsign", "length"))
		var features string
		i.mu.Lock()
		defer i.mu.Unlock()
		if i.hLength >= 1 { //The geojson point of the current location and all the properties
			point := geometry{
				Type:        "Point",
				Coordinates: []float64{i.history[i.hLength-1].long, i.history[i.hLength-1].lat},
			}
			feature1 := feature{
				Type:       "Feature",
				ID:         mmsi,
				Geometry:   point,
				Properties: &properties,
			}
			b1, err := json.MarshalIndent(feature1, "", "\t")
			if err != nil {
				panic(err.Error()) //FIXME
			}
			features = string(b1)

			//Making the LineString object of the ships tracklog (must contain at least 2 points).
			if i.hLength >= 2 {
				c := make([][]float64, 0, i.hLength)
				k := uint16(0)
				for k < i.hLength {
					c = append(c, []float64{i.history[k].long, i.history[k].lat})
					k++
				}
				ls := geometry{
					Type:        "LineString",
					Coordinates: c,
				}
				feature2 := feature{
					Type:       "Feature",
					ID:         mmsi,
					Geometry:   ls,
					Properties: &emptyJsonObject,
				}
				b2, err := json.MarshalIndent(feature2, "", "\t")
				if err != nil {
					panic(err.Error()) //FIXME
				}
				features = features + ", " + string(b2)
			}
		}
		return `{
			"type": "FeatureCollection",
			"features": [` + features + `]}`
	}
	return ""
}

// Matches produces the geojson FeatureCollection containing all the matching ships and some of their properties.
func Matches(matches *[]Match, si *ShipInfo) string { //TODO move this to archive.go instead?
	features := []string{}
	for _, s := range *matches {
		si.rw.RLock()
		i, ok := si.allInfo[s.MMSI]
		si.rw.RUnlock()
		if ok {
			c := []float64{s.Long, s.Lat}
			point := geometry{
				Type:        "Point",
				Coordinates: c,
			}
			prop := json.RawMessage(i.properties("name", "length", "heading"))
			f := feature{
				Type:       "Feature",
				ID:         s.MMSI,
				Geometry:   point,
				Properties: &prop,
			}
			b, err := json.MarshalIndent(f, "", "\t")
			if err != nil {
				panic(err.Error()) //FIXME
			}
			features = append(features, string(b))
		}
	}
	return `{
		"type": "FeatureCollection",
		"features": [` + strings.Join(features, ", ") + `]}`
}

/*
References:
	https://en.wikipedia.org/wiki/Automatic_identification_system#Broadcast_information
	https://golang.org/pkg/encoding/json/
	http://stackoverflow.com/questions/17306358/golang-removing-fields-from-struct-or-hiding-them-in-json-response#17306470
	http://geojsonlint.com/
	http://stackoverflow.com/questions/7933460/how-do-you-write-multiline-strings-in-go#7933487
*/
