var startView = [
    [58.91847, 5.52406],// Stavanger
    [59.05998, 5.93605]
]
var maxShips = 200 // too many points and the browser stops responding
var apiTimeout = 4*1000
var reloadShipsAfter = 1*60*1000
var ships = {} // a cache of all viewed ships
var map = null // Leaflet object
var layer = null // geoJSON layer
var lastBounds = startView

function init() {
    if (typeof L === 'undefined') {// for people who limit what javascript they run
        error("Couldn't load a required library", "leaflet is missing")
        return
    }
    map = L.map('map').fitBounds(startView)
    // world-wide:
    // L.tileLayer('https://api.mapbox.com/styles/v1/sortu/cizziw4s100h22smw6t8lr7b5/tiles/256/{z}/{x}/{y}?access_token={accessToken}', {
    //     accessToken: 'pk.eyJ1Ijoic29ydHUiLCJhIjoiY2l6emhzNmViMDAxeDMycGZ0YXliZDQyOSJ9.Upft9dNldyEZfN2cDnDkIA',
    //     attribution: 'Map data &copy; <a href="http://openstreetmap.org">OpenStreetMap</a> contributors, <a href="http://creativecommons.org/licenses/by-sa/2.0/">CC-BY-SA</a>, Imagery © <a href="http://mapbox.com">Mapbox</a>',
    //     maxZoom: 13
    // }).addTo(map)
    // only covers Norway:
    new L.TileLayer.WMS('https://opencache.statkart.no/gatekeeper/gk/gk.open', {
        layers: 'sjokartraster',
        format: 'image/png',
        attribution: '&copy; <a href="http://kartverket.no/">Kartverket</a>',
        maxZoom: 13
    }).addTo(map)
    layer = L.geoJSON(null, {
        onEachFeature: function(ship, layer) {
           layer.bindPopup(""+ship.properties.name+": "+ship.properties.length+"m")
        }
    }).addTo(map)
    map.on('moveend', function(e) {
        requestArea(e.target.getBounds())
    })
    map.on('zoomend', function(e) {
        requestArea(e.target.getBounds())
    })
    requestArea(map.getBounds())
}

function requestArea(newBounds) {
    lastBounds = newBounds
    var sw = newBounds.getSouthWest()
    var ne = newBounds.getNorthEast()
    console.log(sw.lat+'x'+sw.lng+', '+ne.lat+'x'+ne.lng)
    callAPI('in_area', sw.lat+'x'+sw.lng+','+ne.lat+'x'+ne.lng, function(ships) {
        // limit the number of points on the map to not slow it down.
        // TODO use https://github.com/Leaflet/Leaflet.markercluster or something
        var text = ""+ships.features.length+" ships in area"
        if (ships.features.length > maxShips) {
            ships.features = ships.features.slice(0, maxShips)
            text = "Showing "+maxShips+" of "+text
        }
        document.getElementById('nships').innerText = text
        layer.clearLayers()
        layer.addData(ships)
    })
    setTimeout(function() {
        if (lastBounds === newBounds) {// view is unchanged
            requestArea(lastBounds)
        }
    }, reloadShipsAfter)
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
