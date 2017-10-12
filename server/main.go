package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tormol/AIS/forwarder"
	l "github.com/tormol/AIS/logger"
	"github.com/tormol/AIS/nmeais"
)

// Log holds the logger instance used throuhgout most of the program.
// It's a global variable because to not need a parameter for it everywhere
// it's written to from in the main package at least.
var Log = l.NewLogger(os.Stderr, l.Info)

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write CPU profile to file")
	memprofile := flag.String("memprofile", "", "write memory profile to file")
	portPrefix := flag.Uint("port-prefix", 80, "listen to port n*100+23 and n*100+80, defaults to 80")
	webdir := flag.String("web-root", "", "where to look for the static files for the webpage, defaults to the current directory or static/")
	help := flag.Bool("h", false, "Print this help and exit")
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		Log.FatalIfErr(err, "create CPU profile file")
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// `log`s default Logger is sometimes written to from http.ServeMux and possibly other places.
	// redirect that to our leveled logger:
	log.SetOutput(Log.WriteAdapter(l.Warning))
	log.SetFlags(0) // Log will add the date and time when wanted

	toArchive := make(chan *nmeais.Message)
	a := NewArchive()    //Archive is used to control the reading and writing of ais info to and from the data structures
	go a.Save(toArchive) //Saves the stream of messages to the Archive
	//Use the Archive to retrieve info about position, tracklog, etc..

	newForwarder := make(chan forwarder.Conn, 20)
	// an empty host listens on all network interfaces
	go HTTPServer(fmt.Sprintf(":%d80", *portPrefix), *webdir, newForwarder, a)
	go forwarder.TCPServer(Log, fmt.Sprintf(":%d23", *portPrefix), newForwarder) // the telnet port
	go forwarder.UDPServer(Log, fmt.Sprintf(":%d23", *portPrefix), newForwarder)

	toForwarder := make(chan []byte)
	go forwarder.Manager(Log, toForwarder, newForwarder)

	sm := NewSourceMerger(Log, toForwarder, toArchive)

	Log.AddPeriodic("main", 1*time.Minute, 1*time.Hour, func(c *l.Composer, _ time.Duration) {
		c.Writeln("Number of ships: %d", a.NumberOfShips())
		c.Writeln("waiting to be registered: %d/%d", len(toArchive), cap(toArchive))
		c.Writeln("waiting to be forwarded: %d/%d", len(toForwarder), cap(toForwarder))
		c.Writeln("waiting to start forwarding: %d/%d", len(newForwarder), cap(newForwarder))
		c.Writeln("source connections: %d", atomic.LoadInt32(&ListenerConnections))
	})

	sources := flag.Args()
	if len(sources) == 0 {
		Log.Fatal("Need at least one AIS source")
	}
	for _, s := range sources {
		Log.Debug("source %s", s)
		name, url, timeout, err := parseSource(s, 5*time.Second)
		if err != nil {
			Log.Fatal("%s", err.Error())
		}
		Read(name, url, timeout, sm)
	}

	signalChan := make(chan os.Signal, 1)
	// Intercept ^C and `timeout`s.
	// SIGPIPE is also received when a TCP raw listener disconnects,
	// and if it was what Log wrote to that broke, nothing can be written anyway.
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	// Here we wait for CTRL-C or some other kill signal
	<-signalChan
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		Log.FatalIfErr(err, "create memory profile file")
		runtime.GC()
		pprof.WriteHeapProfile(f)
		defer f.Close()
	}
	Log.Info("\n...Stopping...")
	Log.RunAllPeriodic()
}

func parseSource(s string, defaultTimeout time.Duration) (
	name, url string, timeout time.Duration, err error,
) {
	beforeURL := strings.Index(s, "=")
	url = s[beforeURL+1:]
	name = url
	timeout = defaultTimeout
	if beforeURL != -1 {
		name = s[:beforeURL]
		beforeConf := strings.Index(s[:beforeURL], ":")
		if beforeConf != -1 {
			name = s[:beforeConf]
			timeout, err = time.ParseDuration(s[beforeConf+1 : beforeURL])
			if err != nil {
				err = fmt.Errorf("Invalid timeout: %s", err.Error())
			}
		}
	}
	return
}
