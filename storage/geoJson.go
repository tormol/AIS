/*This file does all the GeoJSON translation*/
package storage

import (
	"encoding/json"
	"strconv"
	"strings"
)

//Returns the ship's tracklog and properties as a GeoJSON FeatureCollection object (LineString of the history AND a Point of the current position, with the given properties)
func geojson_AllInfo(mmsi uint32, history *[]checkpoint, hLength uint16, properties string) string {
	if hLength <= 1 { // A LineString must contain at least 2 points
		if hLength == 1 { //Return the current location and all the known properties
			return `{
				"type": "FeatureCollection",
				"features": [
					{
						"type": "Feature",
						"id":` + strconv.Itoa(int(mmsi)) + `,
						"geometry": {
								"type": "Point",
								"coordinates": [` + strconv.FormatFloat((*history)[hLength-1].long, 'f', 6, 64) + `, ` + strconv.FormatFloat((*history)[hLength-1].lat, 'f', 6, 64) + `]
							}, 
						"properties": {` + properties + `}
					}
				]
			}`
		} else {
			return ""
		}
	}

	s := make([]string, 0, hLength)
	i := uint16(0)
	for i < hLength {
		s = append(s, "["+strconv.FormatFloat((*history)[i].long, 'f', 6, 64)+", "+strconv.FormatFloat((*history)[i].lat, 'f', 6, 64)+"]") //GeoJSON uses <long, lat> instead of <lat, long> ...
		i++
	}
	//could also add the properties to the LineString-object if needed
	return `{
		"type": "FeatureCollection",
		"features": [
		{
			"type": "Feature",
			"id":` + strconv.Itoa(int(mmsi)) + `,
			"geometry": {
					"type": "Point",
					"coordinates": [` + strconv.FormatFloat((*history)[hLength-1].long, 'f', 6, 64) + `, ` + strconv.FormatFloat((*history)[hLength-1].lat, 'f', 6, 64) + `]
				}, 
			"properties": {` + properties + `}
		},
		{
			"type": "Feature",
			"id":` + strconv.Itoa(int(mmsi)) + `,
			"geometry": {
					"type": "LineString",
					"coordinates": [` + strings.Join(s, ", ") + `]
				},
			"properties": {}
		}	
		]
	}`
}

//Returns a GeoJSON FeatureCollection of the matches
func MatchesToGeojson(matches *[]Ship, si *ShipInfo) string { //TODO change si.GetFeatures to return the geojson "properties": {...} ?
	features := []string{}
	var name string
	var length, heading uint16
	for _, s := range *matches {
		name, length, heading = si.GetFeatures(s.MMSI)
		name, _ := json.Marshal(name)
		features = append(features, `{
				"type": "Feature", 
				"id": `+strconv.Itoa(int(s.MMSI))+`,  
				"geometry": { 
					"type": "Point",  
					"coordinates": `+"["+strconv.FormatFloat(s.Long, 'f', 6, 64)+", "+strconv.FormatFloat(s.Lat, 'f', 6, 64)+"]"+`},
				"properties": {
					"name": `+string(name)+`,
					"length": `+strconv.Itoa(int(length))+`,
					"heading": `+strconv.Itoa(int(heading))+`
				}
					
			}`)
	}
	return "{ \"type\": \"FeatureCollection\", \"features\": [" + strings.Join(features, ", ") + "]}"
}

// Returns information about the ship and its tracklog, in GeoJSON
func geojson_ShipProperties(heading, length uint16, name, dest, callsign string) string { //TODO only return the not-nil properties
	return `"name":"` + name + `",
			"destination":"` + dest + `",
			"heading":` + strconv.Itoa(int(heading)) + `,
			"callsign":"` + callsign + `",
			"length":` + strconv.Itoa(int(length))
}

/*
References:
	[1]	http://geojsonlint.com/
	[2]	http://stackoverflow.com/questions/7933460/how-do-you-write-multiline-strings-in-go#7933487
*/
