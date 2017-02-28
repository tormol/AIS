package main

import (
	"fmt" //for debugging //TODO Remove
	"time"

	"github.com/AIS/storage" // use local import?
	ais "github.com/andmarios/aislib"
)

//go Save(toForwarder) ?	should recieve *Message objects (no duplicates)
func Save(msg chan *Message, rt *storage.RTree, si *storage.ShipInfo) {
	counter := 0 //TODO Remove
	for {
		select {
		case m := <-msg:
			var err error
			mmsi := uint32(0)
			ps := (*ais.PositionReport)(nil)
			switch m.Type {
			case 1, 2, 3: // class A position report (longest)
				cApr, e := ais.DecodeClassAPositionReport(m.ArmoredPayload())
				ps = &cApr.PositionReport
				if e != nil || !okCoords(ps.Lat, ps.Lon) {
					continue //TODO
				}
				mmsi = ps.MMSI
				if si.IsKnown(mmsi) {
					oldLat, oldLong := si.GetCoords(mmsi)
					rt.Update(mmsi, oldLat, oldLong, ps.Lat, ps.Lon)
				} else {
					rt.InsertData(ps.Lat, ps.Lon, mmsi)
					if err != nil {
						fmt.Printf("ERROR: <%f, %f>, mmsi: %d, error: %v\n", ps.Lat, ps.Lon, ps.MMSI, err)
					}
				}
				err = si.AddCheckpoint(ps.MMSI, ps.Lat, ps.Lon, time.Now(), ps.Heading)
			case 5: // static voyage data
				svd, e := ais.DecodeStaticVoyageData(m.ArmoredPayload())
				if e != nil && svd.MMSI <= 0 {
					continue //TODO
				}
				err = si.UpdateSVD(svd.MMSI, svd.Callsign, svd.Destination, svd.VesselName, svd.ToBow, svd.ToStern)
			case 18: // basic class B position report (shorter)
				cBpr, e := ais.DecodeClassBPositionReport(m.ArmoredPayload())
				ps = &cBpr.PositionReport
				if e != nil || !okCoords(ps.Lat, ps.Lon) {
					continue //TODO
				}
				mmsi = ps.MMSI
				if si.IsKnown(mmsi) {
					oldLat, oldLong := si.GetCoords(mmsi)
					rt.Update(mmsi, oldLat, oldLong, ps.Lat, ps.Lon)
				} else {
					rt.InsertData(ps.Lat, ps.Lon, mmsi)
				} //TODO check for error?
				err = si.AddCheckpoint(ps.MMSI, ps.Lat, ps.Lon, time.Now(), ps.Heading)
			}
			if err != nil {
				fmt.Printf("Had an error... %v\n", err)
				continue //TODO do something...
			}
			counter++             //TODO Remove
			if counter%100 == 0 { //TODO Remove
				fmt.Printf("Number of boats: %d\n", rt.NumOfBoats())
				//fmt.Println(si.GetHistory(mmsi))
				r, _ := storage.NewRectangle(59.0, 5.54, 59.15, 5.8)
				fmt.Println(rt.FindWithin(r))
				//fmt.Println(rt.FindAll())
			}
		}
	}
}

//Check if the coordinates are ok.	(<91, 181> seems to be a fallback value for the coordinates)
func okCoords(lat, long float64) bool {
	if lat <= 90 && long <= 180 && lat >= -90 && long >= -180 {
		return true
	}
	return false
}
