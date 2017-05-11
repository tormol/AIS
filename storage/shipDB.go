// Structures used to store information and history for each ship.
package storage

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	ais "github.com/andmarios/aislib"
	"github.com/tormol/AIS/geo"
	//"github.com/tormol/AIS/logger"
)

type Mmsi uint32

// CountryCode returns the country identified by the "Maritime Identification Digits" of the mmsi.
func (m Mmsi) CountryCode() string {
	s := m.String()
	a := strings.Split(s, ",")
	if len(a) > 1 {
		return a[1]
	}
	return " - "
}

// Owner returns the type of the owner of the Mmsi.
// E.g. "Ship", "Coastal Station", "MOB —Man Overboard Device", etc.
func (m Mmsi) Owner() string {
	s := m.String()
	a := strings.Split(s, ",")
	if len(a) > 1 {
		return a[0]
	}
	return " - "
}

// String returns the string representation of the Mmsi-object.
// E.g. "Ship, Norway" or "Coastal Station, France".
func (m *Mmsi) String() string {
	return ais.DecodeMMSI(uint32(*m))
}

// ShipNavStatus contains the navigation status code.
// E.g. "Under way using engine", "At anchor", "Not under command", etc.
type ShipNavStatus uint8

// String() returns the navigation status as a string.
func (s *ShipNavStatus) String() string {
	if int(*s) < len(ais.NavigationStatusCodes) {
		return ais.NavigationStatusCodes[uint8(*s)]
	}
	return ""
}

// MarshalJSON() is used by the json Marshaler.
// The json value of the ShipNavStatus-object is the navigation status as a string.
func (s *ShipNavStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Stopped returns true if the ship is "At anchor" or "Moored".
func (s *ShipNavStatus) Stopped() bool {
	if uint8(*s) == 1 || uint8(*s) == 5 {
		return true
	}
	return false
}

// ShipType contains the ship type code.
// E.g. "Fishing", "Sailing", "Tug", "Cargo", etc.
type ShipType uint8

// String returns the ship type as a string.
func (t *ShipType) String() string {
	v, _ := ais.ShipType[int(*t)]
	return v
}

// MarshalJSON is used by the json Marshaler
// The json value of the ShipType-object is the ship type as a string.
func (t ShipType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// Accuracy contains the accuracy of the ships position.
type Accuracy bool

// String returns the string representation of the accuracy (either high or low).
func (a Accuracy) String() string {
	if a {
		return "High accuracy (<10m)"
	}
	return "Low accuracy (>10m)"
}

// MarshalJSON is used by the json Marshaler.
// The json value of the Accuracy-object is the string description of the accuracy.
func (a Accuracy) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// Geometry is used to create GeoJSON "geometry" fields.
// Works for both GeoJSON "Point" and "LineString" objects.
type Geometry struct {
	Coordinates []geo.Point `json:"coordinates"`
}

// MarshalJSON returns either a GeoJSON "Point" or a GeoJSON "LineString" object.
func (g Geometry) MarshalJSON() ([]byte, error) {
	if len(g.Coordinates) >= 2 { // GeoJSON "LineString"
		c, _ := json.Marshal(g.Coordinates)
		b := []byte{'{', '"', 't', 'y', 'p', 'e', '"', ':', '"', 'L', 'i', 'n', 'e', 'S', 't', 'r', 'i', 'n', 'g', '"', ',', '"', 'c', 'o', 'o', 'r', 'd', 'i', 'n', 'a', 't', 'e', 's', '"', ':'}
		b = append(b, c...)
		return append(b, '}'), nil
	} else if len(g.Coordinates) == 1 { // GeoJSON "Point"
		c, _ := json.Marshal(g.Coordinates[0])
		b := []byte{'{', '"', 't', 'y', 'p', 'e', '"', ':', '"', 'P', 'o', 'i', 'n', 't', '"', ',', '"', 'c', 'o', 'o', 'r', 'd', 'i', 'n', 'a', 't', 'e', 's', '"', ':'}
		b = append(b, c...)
		return append(b, '}'), nil
	}
	return []byte{}, errors.New("Not enough coordinates")
}

// ShipPos stores information gathered from AIS message type 1-3, 18-19 and 27.
type ShipPos struct {
	At          time.Time     `json:"time,omitempty"`     // Calculated from UTCSecond and time packet was received
	Pos         geo.Point     `json:"position"`           // A GeoJSON object must have a position, therefore this field can not be omitted
	PosAccuracy Accuracy      `json:"accuracy,omitempty"` // High or low
	NavStatus   ShipNavStatus `json:"navstatus,omitempty"`
	BowHeading  uint16        `json:"heading,omitempty"`    // in degrees with zero north
	Course      float32       `json:"cog,omitempty"`        // Direction of movement, in degrees with zero north
	Speed       float32       `json:"sog,omitempty"`        // in knots
	RateOfTurn  float32       `json:"rateofturn,omitempty"` // in degrees/minute
}

// Should have been const, but math.NaN() is a function and
// 0.0/0.0 (or any indirection thereof) gives a division by zero error.
// This is intentional: https://github.com/golang/go/issues/2196#issuecomment-66058380
var UnknownPos = ShipPos{
	Pos:         geo.Point{math.NaN(), math.NaN()},
	PosAccuracy: false,
	NavStatus:   ShipNavStatus(15),
	BowHeading:  uint16(math.NaN()),
	Course:      float32(math.NaN()),
	Speed:       float32(math.NaN()),
	RateOfTurn:  float32(math.NaN()),
}

// ShipInfo stores information gathered from AIS message 5 and 24.
type ShipInfo struct {
	Draught      uint8     `json:"draught,omitempty"`
	Length       uint16    `json:"length,omitempty"`
	Width        uint16    `json:"width,omitempty"`
	LengthOffset int16     `json:"lengthoffset,omitempty"` // from center
	WidthOffset  int16     `json:"widthoffset,omitempty"`  // from center
	Callsign     string    `json:"callSign,omitempty"`
	ShipName     string    `json:"name,omitempty"`
	VesselType   ShipType  `json:"vesseltype,omitempty"`
	Dest         string    `json:"destination,omitempty"`
	ETA          time.Time `json:"eta,omitempty"`
}

// Should have been const, but arrays aren't: https://groups.google.com/forum/#!topic/golang-nuts/VDaHVzu-D4E
var UnknownInfo = ShipInfo{
	Draught:      0,
	Length:       0,
	Width:        0,
	LengthOffset: 0,
	WidthOffset:  0,
	VesselType:   ShipType(0),
	// strings are all nul, ETA is the zero value
}

// ship contains all the information about a specific mmsi.
type ship struct {
	MMSI     uint32      `json:"mmsi"`
	Owner    string      `json:"owner"`   // The type of the owner of the ship (decoded from the mmsi)
	Country  string      `json:"country"` // The ships country (decoded from the mmsi)
	ShipInfo             // Contains the static information about the ship
	ShipPos              // Contains information about the current position, speed, heading, etc.
	history  []geo.Point // Stores the ship's tracklog
	hLength  uint16      // current number of checkpoints in the history
	mu       *sync.Mutex
}

const HISTORY_MAX = 100 // The maximum number of points allowed to be stored in the history
const HISTORY_MIN = 60  // The minimum number of points stored in the history

// ShipDB contains all the ships.
type ShipDB struct {
	ships map[uint32]*ship
	rw    *sync.RWMutex
}

// NewShipInfo creates and returns a pointer to a new ShipInfo object.
func NewShipDB() *ShipDB {
	return &ShipDB{make(map[uint32]*ship), &sync.RWMutex{}}
}

// Known returns true if the given mmsi is stored in the structure.
func (db *ShipDB) Known(mmsi uint32) bool {
	db.rw.RLock()
	_, ok := db.ships[mmsi]
	db.rw.RUnlock()
	return ok
}

// get takes the mmsi as input and returns the corresponding ship.
func (db *ShipDB) get(mmsi uint32) *ship {
	db.rw.RLock()
	s, _ := db.ships[mmsi]
	db.rw.RUnlock()
	return s
}

// addShip creates a new ship object in the map, and returns a pointer to it.
func (db *ShipDB) addShip(mmsi uint32) *ship {
	// Creating the new ship-object
	newS := &ship{
		mmsi,
		Mmsi(mmsi).Owner(),
		Mmsi(mmsi).CountryCode(),
		UnknownInfo,
		UnknownPos,
		make([]geo.Point, HISTORY_MAX),
		0,
		&sync.Mutex{},
	}
	db.rw.Lock()
	// Check that it doesnt overwrite some other value.
	s, ok := db.ships[mmsi]
	if !ok {
		db.ships[mmsi] = newS
		s = newS
	}
	db.rw.Unlock()
	return s
}

// UpdateStatic updates the ship's static information.
func (db *ShipDB) UpdateStatic(mmsi uint32, update ShipInfo) {
	s := db.get(mmsi)
	if s == nil {
		s = db.addShip(mmsi)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ShipInfo = update
}

// UpdateDynamic updates the ship's dynamic information.
func (db *ShipDB) UpdateDynamic(mmsi uint32, update ShipPos) {
	s := db.get(mmsi)
	if s == nil {
		s = db.addShip(mmsi)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check that the updated information is newer than the current info.
	if update.At.After(s.At) {
		s.ShipPos = update
		// Checking if the ship is moored or anchored
		if !update.NavStatus.Stopped() || s.hLength < 2 { //If the tracklog is too short it is updated regardless of the ship's navigational status.
			s.history[s.hLength] = geo.Point{update.Pos.Lat, update.Pos.Long}
			s.hLength++
			if s.hLength >= HISTORY_MAX { //purge the slice
				newH := make([]geo.Point, HISTORY_MAX)
				copy(newH, s.history[:HISTORY_MIN])
				s.history = newH
				s.hLength = HISTORY_MIN
			}
		}
	}
}

// Coords returns the coordinates of the ship.
func (db *ShipDB) Coords(mmsi uint32) (lat, long float64) {
	s := db.get(mmsi)
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		lat = s.Pos.Lat
		long = s.Pos.Long
	}
	return
}

// GeoJSON Feature structure.
type feature struct {
	Type       string           `json:"type"`
	ID         uint32           `json:"id"`
	Geometry   Geometry         `json:"geometry"`
	Properties *json.RawMessage `json:"properties"`
}

var emptyJsonObject = json.RawMessage(`{}`) //empty struct

// Select returns the info about the ship and its tracklog as a geojson FeatureCollection object.
func (db *ShipDB) Select(mmsi uint32) string {
	s := db.get(mmsi)
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	prop := json.RawMessage(p)
	var features string
	if s.hLength >= 1 { //The geojson point of the current location and all the properties
		feature1 := feature{
			Type:       "Feature",
			ID:         mmsi,
			Geometry:   Geometry{[]geo.Point{s.Pos}}, // The geojson geometry field,
			Properties: &prop,
		}
		b1, err := json.Marshal(feature1)
		if err != nil {
			return ""
		}
		features = string(b1)

		//Making the LineString object of the ships tracklog (must contain at least 2 points).
		if s.hLength >= 2 {
			feature2 := feature{
				Type:       "Feature",
				ID:         uint32(mmsi),
				Geometry:   Geometry{s.history[:s.hLength]},
				Properties: &emptyJsonObject,
			}
			b2, err := json.Marshal(feature2)
			if err != nil {
				return ""
			}
			features = features + ",\n" + string(b2)
		}
	}
	return `{"type": "FeatureCollection","features": [` + features + `]}`
}

// Contains a set of "name, height" values.
// Used in the "properties" field of the GeoJSON object of a Match.
type mProp struct {
	Name   string `json:"name,omitempty"`
	Length uint16 `json:"length,omitempty"`
}

// Matches produces the geojson FeatureCollection containing all the matching ships along with the length and name of the ship.
func Matches(matches *[]Match, db *ShipDB) string { //TODO move this to archive.go instead?
	features := []string{}
	for _, m := range *matches {
		s := db.get(m.MMSI)
		if s != nil {
			point := Geometry{[]geo.Point{geo.Point{m.Lat, m.Long}}}
			s.mu.Lock()
			p, err := json.Marshal(mProp{s.ShipName, s.Length})
			s.mu.Unlock()
			if err != nil {
				continue //skip this ship
			}
			prop := json.RawMessage(p)
			f := feature{
				Type:       "Feature",
				ID:         m.MMSI,
				Geometry:   point,
				Properties: &prop,
			}
			b, err := json.Marshal(f)
			if err != nil {
				continue //skip this ship
			}
			features = append(features, string(b))
		}
	}
	return `{"type": "FeatureCollection","features": [` + strings.Join(features, ",\n") + `]}`
}

/*
References:
	https://en.wikipedia.org/wiki/Automatic_identification_system#Broadcast_information
	https://golang.org/pkg/encoding/json/
	http://geojsonlint.com/
*/
