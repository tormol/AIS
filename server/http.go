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

// DefaultStaticDir should be relative to the current directory and without a trailing slash.
const DefaultStaticDir = "static"

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

func inArea(w http.ResponseWriter, r *http.Request, params string, db *Archive) {
	if r.Method != "GET" {
		writeError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	// parse coordinates
	minLon, minLat, maxLon, maxLat := math.NaN(), math.NaN(), math.NaN(), math.NaN()
	// I want to error on trailing characters, but Sscanf() ignores everything after the
	// pattern. My workaround is to add an extra catch-anything (except empty) pattern, and
	// looking at the number of successfully parsed valuss.
	var remainder string
	parsed, _ := fmt.Sscanf(params, "%f,%f,%f,%f%s", &minLon, &minLat, &maxLon, &maxLat, &remainder)
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
		} // permission errors are unexpected inside DefaultStaticDir
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
func HTTPServer(on string, staticDir string, newForwarder chan<- forwarder.Conn, db *Archive) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/raw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/plain; charset=ascii")
			forwarder.ToHTTP(newForwarder, w, r)
		} else {
			writeError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	})
	mux.HandleFunc("/api/v1/in_area", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.RequestURI, "/api/v1/in_area?bbox=") {
			inArea(w, r, r.RequestURI[len("/api/v1/in_area?bbox="):], db)
		} else {
			writeError(w, r, http.StatusNotFound, "bbox parameter required")
		}
	})
	// "?bbox="" is the norm for such APIs, but IMO "/" is cleaner, so allow that too
	mux.HandleFunc("/api/v1/in_area/", func(w http.ResponseWriter, r *http.Request) {
		params := r.RequestURI[len("/api/v1/in_area/"):]
		params = strings.TrimPrefix(params, "?bbox=")
		inArea(w, r, params, db)
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
		json := db.Select(uint32(mmsi))
		if json == "" {
			writeError(w, r, http.StatusNotFound, "No ship with that MMSI")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeAll(w, r, []byte(json), "with_mmsi JSON")
	})
	if staticDir == "" {
		if _, err := os.Stat(DefaultStaticDir + "/index.html"); !os.IsNotExist(err) {
			staticDir = DefaultStaticDir
		} else {
			staticDir = "."
		}
	} else if strings.HasSuffix(staticDir, "/") {
		staticDir = staticDir[:len(staticDir)-1]
	}
	if _, err := os.Stat(staticDir + "/index.html"); os.IsNotExist(err) {
		Log.Warning("No index.html in %s/", staticDir)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// http.ServeFile doesn't support custom 404 pages,
		// so echoStaticFile and this reimplements most of it.
		if strings.HasSuffix(r.RequestURI, "/index.html") {
			l := len(r.RequestURI) - len("index.html")
			http.Redirect(w, r, r.RequestURI[:l], http.StatusPermanentRedirect)
			return
		}
		if strings.HasSuffix(r.RequestURI, "/") {
			echoStaticFile(w, r, staticDir+r.RequestURI+"index.html")
		} else {
			// if the URI contains '?', let it 404
			echoStaticFile(w, r, staticDir+r.RequestURI)
		}
	})
	err := http.ListenAndServe(on, mux)
	Log.Fatal("HTTP server: %s", err.Error())
}
