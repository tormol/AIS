package main

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/tormol/AIS/forwarder"
)

// StaticRootDir should be relative to the current directory and without a trailing slash.
const StaticRootDir = "static"

func writeAll(w http.ResponseWriter, r *http.Request, data []byte, what string) {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			Log.Info("IO error serving %s to %s: %s", what, r.Host, err.Error())
			return
		}
		data = data[n:]
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, desc string) {
	var content string
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Add("Content-type", "application/json")
		content = `{"error":"` + desc + `"}`
	} else {
		w.Header().Add("Content-type", "text/html; charset=UTF-8")
		content = `<!DOCTYPE html><html lang="en">` +
			`<head><title>` + strconv.Itoa(status) + `</title></head>` +
			`<body><h1>` + desc + `</h1><hr/><a href="/">Go to front page</a></body>` +
			`</html>`
	}
	w.WriteHeader(status)
	if r.Method != "HEAD" {
		writeAll(w, r, []byte(content), desc)
	}
}

func echoStaticFile(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != "GET" && r.Method != "HEAD" {
		writeError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if strings.Contains(path, "/.") || strings.Contains(path, "\\.") {
		// Prevents /../ and hides dot-files left in by accident
		writeError(w, r, http.StatusForbidden, "Forbidden")
		return
	}
	stat, err := os.Stat(path)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "Not found")
		if !os.IsNotExist(err.(*os.PathError).Err) { // docs guarantee it's a *PathError
			Log.Warning("Unexpected os.Stat(\"%s\") error: %s",
				path, err.(*os.PathError).Error())
		} // permission errors are unexpected inside StaticRootDir
		return
	}
	if !stat.Mode().IsRegular() { // directory or something else
		writeError(w, r, http.StatusForbidden, "Forbidden")
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "Not found")
		Log.Warning("os.Open(\"%s\") error after successful stat: %s",
			path, err.(*os.PathError).Error())
		return
	}
	// ServeContent modifies headers, so don't write them.
	http.ServeContent(w, r, path, stat.ModTime(), f)
	err = f.Close()
	if err != nil {
		Log.Error("\"%s\".Close() error: %s", path, err.Error())
	}
}

// HTTPServer starts the HTTP server and never returns.
// For static files to be found, the server must be launched in the parent of StaticRootDir.
func HTTPServer(on string, newForwarder chan<- forwarder.Conn, db *Archive) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/raw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			forwarder.ToHTTP(newForwarder, w, r)
		} else {
			writeError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	})
	mux.HandleFunc("/api/v1/in_area/", func(w http.ResponseWriter, r *http.Request) {
		params := r.RequestURI[len("/api/v1/in_area/"):]
		if r.Method != "GET" {
			writeError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		// parse coordinates
		minLat, minLon, maxLat, maxLon := math.NaN(), math.NaN(), math.NaN(), math.NaN()
		// I want to error on trailing characters, but Sscanf() ignores everything after the
		// pattern. My workaround is to add an extra catch-anything (except empty) pattern, and
		// looking at the number of successfully parsed valuss.
		var remainder string
		parsed, _ := fmt.Sscanf(params, "%fx%f,%fx%f%s", &minLat, &minLon, &maxLat, &maxLon, &remainder)
		if parsed != 4 {
			writeError(w, r, http.StatusBadRequest, "Malformed coordinates")
			return
		}
		json, err := db.FindWithin(minLat, minLon, maxLat, maxLon)
		if err != nil { // out of range or min > max (FIXME rectangles crossing the date line)
			writeError(w, r, http.StatusBadRequest, "Malformed coordinates")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeAll(w, r, []byte(json), "in_area JSON")
	})
	mux.HandleFunc("/api/v1/with_mmsi/", func(w http.ResponseWriter, r *http.Request) {
		params := r.RequestURI[len("/api/v1/with_mmsi/"):]
		if r.Method != "GET" {
			writeError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		mmsi, err := strconv.Atoi(params)
		if err != nil || mmsi <= 0 || mmsi > 999999999 {
			writeError(w, r, http.StatusBadRequest, "Invalid MMSI")
			return
		}
		// TODO return all known fields; if the ship doesn't exist there are none
		writeError(w, r, http.StatusNotImplemented, "TODO")
	})
	mux.HandleFunc("/crash", func(w http.ResponseWriter, r *http.Request) {
		// To make server_runner get a new version from github.
		// FIXME remove this and have server_runner `git fetch`` periodically.
		Log.Fatal("/crash'ed from %s", r.RemoteAddr)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// http.ServeFile doesn't support custom 404 pages,
		// so echoStaticFile and this reimplements most of it.
		if strings.HasSuffix(r.RequestURI, "/index.html") {
			l := len(r.RequestURI) - len("index.html")
			http.Redirect(w, r, r.RequestURI[:l], http.StatusPermanentRedirect)
			return
		}
		if r.RequestURI == "/" {
			// I don't expect multiple directories of static html files
			echoStaticFile(w, r, StaticRootDir+"/index.html")
		} else {
			// if the URI contains '?', let it 404
			echoStaticFile(w, r, StaticRootDir+r.RequestURI)
		}
	})
	err := http.ListenAndServe(on, mux)
	Log.Fatal("HTTP server: %s", err.Error())
}
