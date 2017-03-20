package main

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Relative to current directory, without trailing slash.
const STATIC_ROOT_DIR = "static"

func writeAll(w http.ResponseWriter, r *http.Request, data []byte, what string) {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			Log.Debug("IO error serving %s to %s: %s", what, r.Host, err.Error())
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
		} // permission errors are unexpected inside STATIC_ROOT_DIR
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

// Starts HTTP server.
// For static files to be found, the server must be launched in the parent of STATIC_ROOT_DIR.
// Never returns.
func HttpServer(on string, newForwarder chan<- NewForwarder, db *Archive) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/raw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ForwardRawHTTP(newForwarder, w, r)
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
		min_lat, min_lon, max_lat, max_lon := math.NaN(), math.NaN(), math.NaN(), math.NaN()
		// I want to error on trailing characters, but Sscanf() ignores everything after the
		// pattern. My workaround is to add an extra catch-anything (except empty) pattern, and
		// looking at the number of successfully parsed valuss.
		var remainder string
		parsed, _ := fmt.Sscanf(params, "%fx%f,%fx%f%s", &min_lat, &min_lon, &max_lat, &max_lon, &remainder)
		if parsed != 4 {
			writeError(w, r, http.StatusBadRequest, "Malformed coordinates")
			return
		}
		json, err := db.FindWithin(min_lat, min_lon, max_lat, max_lon)
		if err != nil { // out of range or min > max (FIXME rectangles crossing the date line)
			writeError(w, r, http.StatusBadRequest, "Malformed coordinates")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeAll(w, r, []byte(json), "in_area JSON")
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
			echoStaticFile(w, r, STATIC_ROOT_DIR+"/index.html")
		} else {
			// if the URI contains '?', let it 404
			echoStaticFile(w, r, STATIC_ROOT_DIR+r.RequestURI)
		}
	})
	err := http.ListenAndServe(on, mux)
	Log.Fatal("HTTP server: %s", err.Error())
}
