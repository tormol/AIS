A server that can read AIS messages from multiple sources,repeat a merged stream to clients, and give out recorded information about ships via a (Geo)JSON.  
It also serves a simple website that can present most of the stored information on a map.

# Building instructions

You need version 1.7 or above of the Go compiler; Either install it from a package manger, or download it from https://golang.org/doc/install, 

```sh
go get github.com/tormol/AIS/server # downloads this repo and dependencies
cd $GOPATH/github.com/tormol/AIS
go build -o ais_server server/*.go
```

If you already have the code, download the dependencies with

```sh
go get github.com/andmarios/aislib
go get github.com/cenkalti/backoff
```

If you want to bind to ports below 1024, you can on linux avoid running the entire server as root by using capabilities:

```sh
sudo setcap CAP_NET_BIND_SERVICE=+eip ais_server
./ais_server -port-offset=0
```

# Invocation

The program must be run from the root directory of the repo because it looks for static files for the website in `static/`.

`./ais_server [-port-prefix=NN] [-cpuprofile=file] [-memprofile=file] (source_name(:timeout)=URL | URL) ...`

The source name is used in error messages and logged statistics.  
The timeout is the max duration between packets before the server will reconnect. It must have an unit such as `s`, `ms` or `ns`.  
In the second form, with only the URL, the URL is used as source name, and the timeout defaults to 5s.  
The supported protocols are `http://`, `tcp://` and `file://`. If the protocol is missing `file://` is assumed.  
If the only source is a file, the program will terminate after the end of file is reached.

`-port-prefix` is an offset to the listening port numbers, multiplied by 100.  
The default value is 80, which means the server listen on :8023 for TCP and UDP forwarding, and :8080 for HTTP. Changing the port is necessary to run multiple instances in paralell.
Use `-port-prefix=0` to listen on the standard ports.

`-cpuprofile` and `-memprofile` are supported for profiling, (Go's HTTP interface for profiling is not supported)

If you want to run it on a server, you can adapt the `server_runner` script by setting the variables and directories at the top.

## Example

`./ais_server -port-offset=20 tcp://localhost:3023 kystverket:5s=tcp://153.44.253.27:5631`

# Open realtime data sources

| From | URL | license |
|------|-----|---------|
| [The norwegian coastal administration](http://kystverket.no/Maritime-tjenester/Meldings--og-informasjonstjenester/AIS/Brukartilgang-til-AIS-Norge/) | `tcp://153.44.253.27:5631`| [NLOD](https://data.norge.no/nlod/) (similar to CC-BY) |

# AIS message repeating

*(This section assumes `-port-offset=0`)*  
The merged stream of AIS sentences can be received over the following protocols:
* HTTP: Send a `GET` request to `/api/v1/raw` on port 80.
* TCP: Connect to port 23 (the telnet port).
* UDP: Send packets to the server on the same port as TCP. The server will stop sending after five seconds without receiving any packets,so send more frequently in case some get lost. The content of the packets is ignored.
Each sent Datagram will contain a single, complete AIS message. Use an 1KB read buffer to avoid any trunkation.

You can look at the stream from a terminal with the following commands:
* HTTP: `wget -qO- localhost/api/v1/raw`
* TCP: `nc localhost 23` or `telnet localhost`
* UDP: `nc -u localhost 23` and press enter every few seconds.

# JSON API

## Get all known information about a ship based on its [MMSI](https://en.wikipedia.org/wiki/Maritime_Mobile_Service_Identity)

`/api/v1/with_mmsi/$MMSI`. The MMSI cannot conain spaces or hyphens.
If a ship with the MMSI is known, the response will be a GeoJSON `FeatureCollection` with one or two features: The first is a point with all the properties of the ship:

| name | type | example value | description |
| --- | --- | --- | --- |
| `mmsi` | integer | `258226000` |  |
| `type` | string | `"Ship"` | The type of vessel (based on the MMSI) |
| `country` | string | `" Norway"` | The ships country (based on the MMSI) |
| `time` | integer | `"2017-05-14T11:29:21.481126469Z"` | when the position was received |
| `position` | array | `[5.45386666,59.0470833]` |  |
| `accuracy` | string | `"High accuracy (<10m)"` |  |
| `navstatus` | string | `"Moored"` | NavStatus |
| `heading` | integer | `281` | The direction the ships bow is pointing, in degrees with zero north |
| `cog` | number | `281.9` | Direction of movement, in degrees with zero north |
| `sog` | number | `12.6` | Speed over ground, in knots |
| `rateofturn` | number | `127` | in degrees per minute |
| `vesseltype` | string | `"Passenger"` |  |
| `draught` | integer | `48` | the ships depth in decimeter |
| `length` | integer | `40` |  |
| `width` | integer | `7` |  |
| `lengthoffset` | integer | `13` | The positions offset from the boats midship |
| `widthoffset` | integer | `-1` | The positions offset from the boats centerline |
| `callSign` | string | `"LLLZ"` |  |
| `name` | string | `"FJORDVEIEN"` |  |
| `destination` | string | `"MEKJARVIK-KVITSOY T/"` |  |
| `eta` | string | `"0000-05-07T23:30:00Z"` | Estimated Time to Arrival|

`mmsi`, `type`, `country`, `time` and `position` are always available, other properties are omitted when there is no data.
If more than one position has been recorded for the ship, there will be a second feature: A linestring with the most recent positions of the ship. Beware of the antimeridian.
If there is no ship with the specified MMSI, a 404 respose is returned.

## Get the position and MMSI of all ships within a bounding box

`/api/v1/in_area/$sw_lon,$sw_lat,$ne_lon,$ne_lat` where `sw` stands for south-west and `ne` for north-east. The longitudes and latitudes are in degrees. `/api/v1/in_area?bbox=$sw_lon,$sw_lat,$ne_lon,$ne_lat` is also supported.  
Latitudes must be within [-90,90] and north must be greater than south.
longitudes will be normalized to (-180,180] before searching, boxes that span the date line / antimeridian (where west > east) are supported.  
The ships are returned as GeoJSON `Point`s in a `FeatureCollection`.
The ships name and length is included as properties if known.

## Examples

* Get details for the Mekjavik-Kvitsøy ferry: `/api/v1/with_mmsi/258226000`
* Get all ships: `/api/v1/in_area/-180,-90,180,90`
* ... or with `?bbox=`: `/api/v1/in_area?bbox=-180,-90,180,90`
* Get ships around Stavanger (the default view of the website): `/api/v1/in_area/5.52406,58.91847,5.93605,59.05998`
* ... or offset one time east:`/api/v1/in_area/365.52406,58.91847,365.93605,59.05998`
* Get ships around Fiji: `/api/v1/in_area/176.3,-20.1,180.3,-16.1`
* ... or normalized: `/api/v1/in_area/176.3,-20.1,-179.7,-16.1`

# License

Copyright (C) 2017 Torbjørn Birch Moltu and Ivar Sørbø.
Licensed under version 3 of the GNU Affero General Public License,
see `LICENCE` for details.
