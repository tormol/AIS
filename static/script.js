function init() {
    var map = L.map('map').fitBounds([
        [58.91847, 5.52406],// Stavanger
        [59.05998, 5.93605]
    ]);
    // world-wide:
    // L.tileLayer('https://api.mapbox.com/styles/v1/sortu/cizziw4s100h22smw6t8lr7b5/tiles/256/{z}/{x}/{y}?access_token={accessToken}', {
    //     accessToken: 'pk.eyJ1Ijoic29ydHUiLCJhIjoiY2l6emhzNmViMDAxeDMycGZ0YXliZDQyOSJ9.Upft9dNldyEZfN2cDnDkIA',
    //     attribution: 'Map data &copy; <a href="http://openstreetmap.org">OpenStreetMap</a> contributors, <a href="http://creativecommons.org/licenses/by-sa/2.0/">CC-BY-SA</a>, Imagery © <a href="http://mapbox.com">Mapbox</a>',
    //     maxZoom: 13
    // }).addTo(map);
    // only covers Norway:
    new L.TileLayer.WMS("https://opencache.statkart.no/gatekeeper/gk/gk.open", {
        layers: 'sjokartraster',
        format: 'image/png',
        attribution: '&copy; <a href="http://kartverket.no/">Kartverket</a>',
        maxZoom: 13
    }).addTo(map);
    map.on("moveend", function(e) {
        var b = e.target.getBounds()
        console.log(b.getSouthWest().toString() + ' ' + b.getNorthEast().toString())
    })
}