package main

import (
	"errors"
	"fmt" //for debugging //TODO Remove
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

	si *storage.ShipInfo //Contains tracklog and other info for each ship
}

//Returns a pointer to the new Archive
func NewArchive() *Archive {
	return &Archive{
		rt: storage.NewRTree(),
		rw: &sync.RWMutex{},
		si: storage.NewShipInfo(),
	}
}

// Stores the information recieved form the channel
func (a *Archive) Save(msg chan *nmeais.Message) {
	counter := 0 //TODO Remove
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
					continue //TODO
				}
				err = a.updatePos(ps)
			case 5: // static voyage data
				svd, e := ais.DecodeStaticVoyageData(m.ArmoredPayload())
				if e != nil && svd.MMSI <= 0 {
					continue //TODO
				}
				err = a.si.UpdateSVD(svd.MMSI, svd.Callsign, svd.Destination, svd.VesselName, svd.ToBow, svd.ToStern)
			case 18: // basic class B position report (shorter)
				cBpr, e := ais.DecodeClassBPositionReport(m.ArmoredPayload())
				ps = &cBpr.PositionReport
				if e != nil {
					continue //TODO
				}
				err = a.updatePos(ps)
			}
			if err != nil {
				//fmt.Printf("Had an error saving to Archive... %v\n", err)
				continue //TODO do something...
			}
			counter++              //TODO Remove
			if counter%1000 == 0 { //TODO Remove
				fmt.Printf("Number of boats: %d\n", a.NumberOfShips())
				//fmt.Println(a.FindWithin(59.0, 5.54, 59.15, 5.8))
				fmt.Println(a.GetAllInfo(259216000))
				//fmt.Println(a.FindAll())
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
		return errors.New(fmt.Sprintf("Cannot update position... MMSI: %d, lat: %f, long %f", mmsi, ps.Lat, ps.Lon))
	}
	//Check if it is a known ship
	if a.si.IsKnown(mmsi) {
		oldLat, oldLong := a.si.GetCoords(mmsi) //get the previous coordinates
		a.rw.Lock()
		err := a.rt.Update(mmsi, oldLat, oldLong, ps.Lat, ps.Lon) //update the position in the R*Tree
		if err != nil {
			return errors.New("The archive failed to update the position of the ship")
		}
		a.rw.Unlock()
	} else {
		a.rw.Lock()
		a.rt.InsertData(ps.Lat, ps.Lon, mmsi) //insert a new ship into the R*Tree
		a.rw.Unlock()
	} //TODO check for error?
	err := a.si.AddCheckpoint(ps.MMSI, ps.Lat, ps.Lon, time.Now(), ps.Heading) //Adds the position to the ships tracklog
	return err
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
	r, err := geo.NewRectangle(minLat, minLong, maxLat, maxLong)
	if err != nil {
		return "{}", fmt.Errorf("ERROR, invalid rectangle coordinates")
	}
	a.rw.RLock()
	matchingShips := a.rt.FindWithin(r)
	a.rw.RUnlock()
	return storage.Matches(matchingShips, a.si), nil
}

// Check if the coordinates are ok.	(<91, 181> seems to be a fallback value for the coordinates)
func okCoords(lat, long float64) bool {
	if lat <= 90 && long <= 180 && lat >= -90 && long >= -180 {
		return true
	}
	return false
}

// Returns the information about the ship and its tracklog, in GeoJSON
func (a *Archive) GetAllInfo(mmsi uint32) string {
	if !a.si.IsKnown(mmsi) {
		return ""
	}
	return a.si.GetAllInfo(mmsi)
}
