var startView = [
    [58.91847, 5.52406+360],// Stavanger
    [59.05998, 5.93605+360] // offset for testing
]
var maxShips = 200 // too many points and the browser stops responding
var apiTimeout = 4*1000
var reloadShipsAfter = 1*60*1000
var reloadInfoAfter = 2*60*1000
var ships = {} // a cache of all viewed ships
var map = null // Leaflet object
var layer = null // geoJSON layer
var lastBounds = startView

function init() {
    if (typeof L === 'undefined') {// for people who limit what javascript they run
        error("Couldn't load a required library", "leaflet is missing")
        return
    }
    map = L.map('map', {
        maxBounds: L.latLngBounds(L.latLng(-90, -Infinity), L.latLng(90, Infinity)),
        minZoom: 1,
        maxZoom: 13
    })
    // draw lines on date lines
    for (var i=-5; i<5; i++) {
        var lng = 360*i+180
        L.polyline([L.latLng(-90,lng), L.latLng(90,lng)], {color: "red"}).addTo(map)
    }
    map.fitBounds(startView)
    // world-wide:
    // L.tileLayer('https://api.mapbox.com/styles/v1/sortu/cizziw4s100h22smw6t8lr7b5/tiles/256/{z}/{x}/{y}?access_token={accessToken}', {
    //     accessToken: 'pk.eyJ1Ijoic29ydHUiLCJhIjoiY2l6emhzNmViMDAxeDMycGZ0YXliZDQyOSJ9.Upft9dNldyEZfN2cDnDkIA',
    //     attribution: 'Map data &copy; <a href="http://openstreetmap.org">OpenStreetMap</a> contributors, <a href="http://creativecommons.org/licenses/by-sa/2.0/">CC-BY-SA</a>, Imagery © <a href="http://mapbox.com">Mapbox</a>',
    // }).addTo(map)
    // only covers Norway:
    new L.TileLayer.WMS('https://opencache.statkart.no/gatekeeper/gk/gk.open', {
        layers: 'sjokartraster',
        format: 'image/png',
        attribution: '&copy; <a href="http://kartverket.no/">Kartverket</a>',
    }).addTo(map)
    layer = L.geoJSON(null, {
        onEachFeature: function(ship, layer) {
           layer.bindPopup(""+ship.properties.name+": "+ship.properties.length+"m")
        }
    }).addTo(map)
    map.on('popupopen', function(e) {
        var mmsi = e.popup._source.feature.id
        getShip(mmsi, function(about) {
            var html = "<ul>"
            for (var prop in about) {
                if (about.hasOwnProperty(prop)) {
                    html = html+"<li>"+prop+": "+about[prop]+"</li>"
                }
            }
            html += "</ul>"
            e.popup.setContent(html)
        })
    })
    map.on('moveend', function(e) {
        requestArea(e.target.getBounds())
    })
    map.on('zoomend', function(e) {
        requestArea(e.target.getBounds())
    })
    requestArea(map.getBounds())
}

function getShip(mmsi, callback) {
    // see if we have it
    if (ships[mmsi] && (Date.now()-ships[mmsi].retrieved) < reloadInfoAfter) {
        callback(ships[mmsi].details)
        return
    }
    callAPI("with_mmsi", ''+mmsi, function(about) {
        if (about[mmsi] !== undefined) {
            ships[mmsi] = {
                retrieved: Date.now(),
                details: about[mmsi]
            }
        }
        callback(about[mmsi])
    })
}

function requestArea(newBounds) {
    lastBounds = newBounds
    var sw = newBounds.getSouthWest()
    var ne = newBounds.getNorthEast()
    console.log(sw.lat+'x'+sw.lng+', '+ne.lat+'x'+ne.lng)
    callAPI('in_area', sw.lng+','+sw.lat+','+ne.lng+','+ne.lat, function(ships) {
        // limit the number of points on the map to not slow it down.
        // TODO use https://github.com/Leaflet/Leaflet.markercluster or something
        var text = ""+ships.features.length+" ships in area"
        if (ships.features.length > maxShips) {
            ships.features = ships.features.slice(0, maxShips)
            text = "Showing "+maxShips+" of "+text
        }
        document.getElementById('nships').innerText = text
        if (sw.lng <= -180 || ne.lng > 180) {
            lngOffsetPoints(sw.lng, ne.lng, ships.features)
        }
        layer.clearLayers()
        layer.addData(ships)
    })
    setTimeout(function() {
        if (lastBounds === newBounds) {// view is unchanged
            requestArea(lastBounds)
        }
    }, reloadShipsAfter)
}

// offset point features to match the viewport
function lngOffsetPoints(west,east,points) {
    var westOffset = lngOffset(west)
    var eastOffset = lngOffset(east)
    // console.log("west: "+west+" => "+westOffset+" => "+(west-westOffset))
    // console.log("east: "+east+" => "+eastOffset+" => "+(east-eastOffset))
    // TODO copy if visible multiple places
    if (eastOffset - westOffset === 360) {// one date-line is visible
        // move some one extra
        var swapDivide = west - westOffset
        for (var p of points) {
            if (p.geometry.coordinates[0] < swapDivide) {
                p.geometry.coordinates[0] += eastOffset
            } else {
                p.geometry.coordinates[0] += westOffset
            }
        }
    } else {
        var allOffset = westOffset // all are equally offset
        if (eastOffset - westOffset > 360) {// more than one visible
            allOffset += 360 // center to the leftmost whole world
        }
        if (allOffset !== 0) {
            for (var p of points) {
                p.geometry.coordinates[0] += allOffset
            }
        }
    }
}

// Calculates the multiple of 360 that must be added to a ships longitude to
// move it within view.
// For values in (-180, 180] 0 is returned, -180 is considered an offset 180.
function lngOffset(viewPortLng) {
    // by subtracting 180, normalized coordinates become negative, after
    // dividing by 360 they are in the range (-1,0], which are rounded up to 0.
    return 360*Math.ceil((viewPortLng-180)/360)
}

function error(msg, detailed) {
    console.log(detailed || msg) // when called with a single parameter, display it both places
    document.getElementById('error').innerText = msg
}
function clear_error() {
    document.getElementById('error').innerText = ""
}

function callAPI(part, params, callback) {
    // https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest
    // https://dev.opera.com/articles/xhr2/
    var r = new XMLHttpRequest()
    if (!r || r.onload === undefined) {// IE < 10
        error("Browser not supported")
        return
    }
    r.onload = function() {// request went through and we got a response
        checkAPIResponse(r, part+'/'+params, callback)
    }
    r.onerror = function() {
        error("Cannot connect to server")
        // TODO: find error cause? (the event parameter is just {isTrusted: true})
    }
    r.ontimeout = function() {
        error("Server timed out")
    }
    r.timeout = apiTimeout
    // r.responseType = 'json' breaks if the response gets chunked, and isn't supported by Edge and IE
    r.responseType = 'text'
    r.open('GET', 'api/v1/'+part+'/'+params)
    r.setRequestHeader('Accept', 'application/json')
    r.setRequestHeader('Cache-Control', 'no-cache')
    r.send()
}

function checkAPIResponse(r, query, callback) {
    var json = undefined
    var correctHeader = r.getResponseHeader('Content-Type') === 'application/json'
    try {
        json = JSON.parse(r.responseText)
        if (!correctHeader) { // Log it, but don't be brittle about it
            console.log(query+": Got valid JSON but with wrong header: "
                        +r.getResponseHeader('Content-Type')
                        +" (status code: "+r.status+")")
        }
    } catch (e) {
        if (e.name !== 'SyntaxError') {
            throw e // not a JSON error
        }
        var detailed = "server error: expected application/json, got "
                     + r.getResponseHeader('Content-Type')+":\n"
                     + "status: "+r.status+", content: "+r.responseText
        if (correctHeader) {// Server said JSON but didn't deliver
            detailed = "server sent malformed JSON: "+r.responseText
        }
        error("Server error", query+": "+detailed)
        return
    }

    // https://en.wikipedia.org/wiki/List_of_HTTP_status_codes
    if (r.status === 200) { // the request succeded and the server sent valid JSON:
        clear_error()
        callback(json) // proceed to response handler
        return
    } else if (!json.error) {
        error(
            "Server error",
            query+": no error field in error "+r.status+" response: "+r.responseText
        )
    } else if (r.status >= 500) {
        error(json.error, query+": server error "+r.status+": "+json.error)
    } else if (r.status >= 400) {// the javascript should prevent making bad requests
        error("Programmer error", query+": "+json.error+" (status "+r.status+")")
    } else {
        error("Unknown error", query+": unknown error: "+json.error+" (status "+r.status+")")
    }
}
