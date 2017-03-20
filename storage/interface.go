package storage

import (
	"fmt"
	"math"
	"time"

	"github.com/tormol/AIS/logger"
)

type Mmsi uint32

func (m Mmsi) CountryCode() string {
	panic("TODO")
}
func (m *Mmsi) String() string {
	panic("TODO")
}

type ShipNavStatus uint8

func (s *ShipNavStatus) String() string {
	panic("TODO")
}

type ShipType struct {
	ShipCat   uint8
	HazardCat uint8
}

func (t *ShipType) String() string {
	panic("TODO")
}

// Abstracts over AIS message type 1-3, 18-19 and 27
// The flags accuracy, GNSS status and RAIM are merged and abstracted into accuracy
// Radio status, regional bits, spare bits and various flags of 18 and 19 are ignored for now
type ShipPos struct {
	At          time.Time // Calculated from UTCSecond and time packet was received
	Pos         Point
	PosAccuracy uint16 // in meters
	NavStatus   ShipNavStatus
	BowHeading  float32 // in degrees with zero north
	Course      float32 // Direction of movement, in degrees with zero north
	Speed       float32 // in knots
	RateOfTurn  float32 // in degrees/minute
}

// Only prints known values, and doesn't wrap the output in "{}" (to allow for extension)
func (p *ShipPos) JsonKeys() string {
	panic("TODO")
}
func (p *ShipPos) UpdateFrom(update ShipPos) {
	panic("TODO")
}

// Should have been const, but math.NaN() is a function and
// 0.0/0.0 (or any indirection thereof) gives a division by zero error.
// This is intentional: https://github.com/golang/go/issues/2196#issuecomment-66058380
var UnknownPos = ShipPos{
	Pos:         Point{math.NaN(), math.NaN()},
	PosAccuracy: 0xffff,
	NavStatus:   ShipNavStatus(15),
	BowHeading:  float32(math.NaN()),
	Course:      float32(math.NaN()),
	Speed:       float32(math.NaN()),
	RateOfTurn:  float32(math.NaN()),
}

// Abstraction over AIS message 5 and 24
// arrays are \0-terminated if not full, and empty if unknown
// all lengths are in decimeter.
// Use UnknownInfo to see if value is unknown
type ShipInfo struct {
	Draught        uint16
	Length         uint16
	Width          uint16
	LengthOffset   int16 // from center
	WidthOffset    int16 // from center
	Callsign       [7]byte
	ShipName       [20]byte
	VesselType     ShipType
	MothershipMmsi Mmsi
	Dest           [20]byte
	ETA            time.Time
}

// Only prints known values, and doesn't wrap the output in "{}" (to allow for extension)
func (i *ShipInfo) JsonKeys() string {
	panic("TODO")
}
func (i *ShipInfo) UpdateFrom(update ShipInfo) {
	panic("TODO")
}

// Should have been const, but arrays aren't: https://groups.google.com/forum/#!topic/golang-nuts/VDaHVzu-D4E
var UnknownInfo = ShipInfo{
	Draught:      0xffff,
	Length:       0xffff,
	Width:        0xffff,
	LengthOffset: -0x8000,
	WidthOffset:  -0x8000,
	VesselType: ShipType{
		ShipCat:   0,
		HazardCat: 0,
	},
	MothershipMmsi: Mmsi(0),
	// strings are all nul, ETA is the zero value
}

// A trait to allow ShipDB implementations to add internal fields or fine-grained locking.
type Ship interface {
	Mmsi() Mmsi
	CurrentPos() ShipPos
	Info() ShipInfo
	OldPos(func(saved time.Time, pos ShipPos) bool) // from newest to oldest
}

// A trait to abstract over multiple possible implementations in a thread-safe way.
// Uses callbacks so that the implementation can manage locking; the Ship
// instances should not be used outside the callback.
// Iterators are stopped if the callback returns false,
// Callbacks are slower than `for range`, but you would then need to build the slice too.
// Methods can panic if the operation isn't implemented or supported
type ShipDB interface {
	UpdateShipInfo(Mmsi, ShipInfo)
	UpdateShipPos(Mmsi, ShipPos)
	LookupShip(mmsi Mmsi, viewer func(Ship))
	ViewArea(area Rectangle, iterator func(Ship) bool)
	// if there is time...
	Search(text string, iterator func(Ship) bool)
	// For O(n) searching
	// might have to lock every ship while iterating, which seems like a lot,
	// but many databases use row-level locking too.
	AllShips(iterator func(Ship) bool)
	// Faster than AllShips
	NumberOfShips() int64
	// Returns a geojson representation of the position index structure
	DebugShowLayout() string
}

func NewWhatever(log *logger.Logger, oldSnapshotFile string) (ShipDB, error) {
	if oldSnapshotFile != "" {
		return nil, fmt.Errorf("TODO restart without loosing data")
	}
	panic("TODO move archive.go into this package and return NewArchive()")
}
