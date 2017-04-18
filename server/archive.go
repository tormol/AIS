package main

import (
	"errors"
	"math"
	"sync"
	"time"

	ais "github.com/andmarios/aislib"
	"github.com/tormol/AIS/geo"
	"github.com/tormol/AIS/nmeais"
	"github.com/tormol/AIS/storage"
)

//The Archive stores the information about the ships (and works as a temp. solution for the RTree concurrency)
type Archive struct {
	rt *storage.RTree //Stores the points
	rw *sync.RWMutex  //works as a lock for the RTree (#TODO: RTree should be improved to handle concurrency on its own)

	db *storage.ShipDB //Contains tracklog and other info for each ship
}

//Returns a pointer to the new Archive
func NewArchive() *Archive {
	return &Archive{
		rt: storage.NewRTree(),
		rw: &sync.RWMutex{},
		db: storage.NewShipDB(),
	}
}

// Stores the information recieved form the channel
func (a *Archive) Save(msg chan *nmeais.Message) {
	for {
		select {
		case m := <-msg:
			var err error
			ps := (*ais.PositionReport)(nil)
			switch m.Type() {
			case 1, 2, 3: // class A position report (longest)
				cApr, e := ais.DecodeClassAPositionReport(m.ArmoredPayload())
				ps = &cApr.PositionReport
				if e != nil {
					continue
				}
				err = a.updatePos(ps)
				a.db.UpdateDynamic(ps.MMSI, storage.ShipPos{time.Now(), geo.Point{ps.Lat, ps.Lon}, storage.Accuracy(ps.Accuracy), storage.ShipNavStatus(cApr.Status), ps.Heading, ps.Course, ps.Speed, cApr.Turn})
			case 5: // static voyage data
				svd, e := ais.DecodeStaticVoyageData(m.ArmoredPayload())
				if e != nil && svd.MMSI <= 0 {
					continue
				}
				length := uint16(svd.ToBow + svd.ToStern)
				lOffset := int16(length/2 - svd.ToBow)
				width := uint16(svd.ToPort + svd.ToStarboard)
				wOffset := int16(width/2 - uint16(svd.ToStarboard))
				a.db.UpdateStatic(svd.MMSI, storage.ShipInfo{svd.Draught, length, width, lOffset, wOffset, svd.Callsign, svd.VesselName, storage.ShipType(svd.ShipType), svd.Destination, svd.ETA})
			case 18: // basic class B position report (shorter)
				cBpr, e := ais.DecodeClassBPositionReport(m.ArmoredPayload())
				ps = &cBpr.PositionReport
				if e != nil {
					continue
				}
				err = a.updatePos(ps)
				a.db.UpdateDynamic(ps.MMSI, storage.ShipPos{time.Now(), geo.Point{ps.Lat, ps.Lon}, storage.Accuracy(ps.Accuracy), storage.ShipNavStatus(15), ps.Heading, ps.Course, ps.Speed, float32(math.NaN())})
			}
			if err != nil {
				continue //TODO do something...
			}
		}
	}
}

//Returns the number of known ships:
func (a *Archive) NumberOfShips() int {
	a.rw.RLock()
	defer a.rw.RUnlock()
	return a.rt.NumOfBoats()
}

//Updates the ships position in the structures (message type 1,2,3,18)
func (a *Archive) updatePos(ps *ais.PositionReport) error {
	mmsi := ps.MMSI
	if !okCoords(ps.Lat, ps.Lon) || mmsi <= 0 { //This happends quite frequently (coordinates are set to 91,181)
		return errors.New("Cannot update position")
	}
	//Check if it is a known ship
	if a.db.Known(mmsi) {
		oldLat, oldLong := a.db.Coords(mmsi) //get the previous coordinates
		if oldLat == 0 && oldLong == 0 {
			return errors.New("The ship has no known coordinates")
		}
		a.rw.Lock()
		err := a.rt.Update(mmsi, oldLat, oldLong, ps.Lat, ps.Lon) //update the position in the R*Tree
		a.rw.Unlock()
		if err != nil {
			return errors.New("The archive failed to update the position of the ship")
		}
	} else {
		a.rw.Lock()
		a.rt.InsertData(ps.Lat, ps.Lon, mmsi) //insert a new ship into the R*Tree
		a.rw.Unlock()
	}
	return nil
}

// Returns a GeoJSON FeatureCollection containing all the known ships
func (a *Archive) FindAll() string {
	geoJsonFC, _ := a.FindWithin(-79.999999, -179.999999, 79.999999, 179.999999)
	return geoJsonFC
}

/*
Public func for finding all known boats that overlaps a given rectangle of the map [13], [14]
	input:
		minLatitude, minLongitude, maxLatitude, maxLongitude	float64
	output:
		string	-	All matching ships in GeoJSON FeatureCollection

*/
func (a *Archive) FindWithin(minLat, minLong, maxLat, maxLong float64) (string, error) {
	rects := geo.SplitViewRect(minLat, minLong, maxLat, maxLong)
	if rects == nil {
		return "{}", errors.New("ERROR, invalid rectangle coordinates")
	}
	matches := []storage.Match{}
	a.rw.RLock()
	for _, r := range rects {
		m := a.rt.FindWithin(&r)
		matches = append(matches, *m...)
	}
	a.rw.RUnlock()
	// TODO return rectangles?
	return storage.Matches(&matches, a.db), nil
}

// Check if the coordinates are ok.	(<91, 181> seems to be a fallback value for the coordinates)
func okCoords(lat, long float64) bool {
	if lat <= 90 && long <= 180 && lat >= -90 && long >= -180 {
		return true
	}
	return false
}

// Returns the information about the ship and its tracklog, in GeoJSON
func (a *Archive) Select(mmsi uint32) string {
	if !a.db.Known(mmsi) {
		return ""
	}
	return a.db.Select(mmsi)
}
