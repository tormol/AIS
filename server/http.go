package main

import (
	"net/http"
	"os"
	"strings"
)

// Relative to current directory, without trailing slash.
const STATIC_ROOT_DIR = "static"

const RESP_404 = `<!DOCTYPE html><html lang="en">
<head><title>404</title></head>
<body><h1>Page not found</h1><hr/><a href="/">Go to front page</a>`
const RESP_403 = `<!DOCTYPE html><html lang="en">
<head><title>403</title></head>
<body><h1>Forbidden</h1>`

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Conten-type", "text/html")
	w.WriteHeader(http.StatusNotFound)
	_, err := w.Write([]byte(RESP_404))
	if err != nil {
		Log.Debug("Error writing 404 response to %s: %s",
			r.Host, err.Error())
	}
}

func writeForbidden(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Conten-type", "text/html")
	w.WriteHeader(http.StatusForbidden)
	_, err := w.Write([]byte(RESP_403))
	if err != nil {
		Log.Debug("Error writing 403 response to %s: %s",
			r.Host, err.Error())
	}
}

func echoStaticFile(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if strings.Contains(path, "/.") || strings.Contains(path, "\\.") {
		// Prevents /../ and hides dot-files left in by accident
		writeForbidden(w, r)
		return
	}
	stat, err := os.Stat(path)
	if err != nil {
		writeNotFound(w, r)
		if !os.IsNotExist(err.(*os.PathError).Err) { // docs guarantee it's a *PathError
			Log.Warning("Unexpected os.Stat(\"%s\") error: %s",
				path, err.(*os.PathError).Error())
		} // permission errors are unexpected inside STATIC_ROOT_DIR
		return
	}
	if !stat.Mode().IsRegular() { // directory or something else
		writeForbidden(w, r)
	}
	f, err := os.Open(path)
	if err != nil {
		writeNotFound(w, r)
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
func HttpServer(on string, newForwarder chan<- NewForwarder) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ais/raw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ForwardRawHTTP(newForwarder, w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed) // not even HEAD
		}
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
