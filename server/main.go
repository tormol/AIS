package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/tormol/AIS/logger"
	"github.com/tormol/AIS/nmeais"
)

var (
	// Log is the default logger instance. It's a global variable to make it easy to write to.
	Log = NewLogger(os.Stderr, LOG_DEBUG, 10*time.Second)
	// For input sentence or message "errors"
	AisLog = NewLogger(os.Stdout, LOG_DEBUG, 10*time.Second)
)

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	portPrefix := flag.Uint("port-prefix", 80, "listen to port this*100+23 and this*100+80, default is 80")
	help := flag.Bool("h", false, "Print this help and exit")
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			Log.Fatal("%s", err.Error())
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// `log`s default Logger is sometimes written to from http.ServeMux and possibly other places.
	// redirect that to our leveled logger:
	log.SetOutput(Log.WriteAdapter(LOG_WARNING))
	log.SetFlags(0) // Log will add the date and time when wanted

	toArchive := make(chan *nmeais.Message)
	a := NewArchive()    //Archive is used to control the reading and writing of ais info to and from the data structures
	go a.Save(toArchive) //Saves the stream of messages to the Archive
	//Use the Archive to retrieve info about position, tracklog, etc..

	newForwarder := make(chan NewForwarder, 20)
	// an empty host listens on all network interfaces
	go HttpServer(fmt.Sprintf(":%d80", *portPrefix), newForwarder, a)
	go ForwardRawTCPServer(fmt.Sprintf(":%d23", *portPrefix), newForwarder) // the telnet port
	go ForwardRawUDPServer(fmt.Sprintf(":%d23", *portPrefix), newForwarder)

	toForwarder := make(chan []byte)
	go ForwarderManager(toForwarder, newForwarder)

	sm := NewSourceMerger(Log, toForwarder, toArchive)

	Log.AddPeriodicLogger("from_main", 120*time.Second, func(l *Logger, _ time.Duration) {
		c := l.Compose(LOG_DEBUG)
		c.Writeln("waiting to be registered: %d/%d", len(toArchive), cap(toArchive))
		c.Writeln("waiting to be forwarded: %d/%d", len(toForwarder), cap(toForwarder))
		c.Writeln("waiting to start forwarding: %d/%d", len(newForwarder), cap(newForwarder))
		c.Writeln("source connections: %d", atomic.LoadInt32(&Listener_connections))
		c.Close()
	})

	sources := flag.Args()
	if len(sources) == 0 {
		sources = append(sources, "ECC:5s=http://aishub.ais.ecc.no/raw")
		sources = append(sources, "kystverket:5s=tcp://153.44.253.27:5631")
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
	_ = <-signalChan
	Log.Info("\n...Stopping...")
	Log.RunPeriodicLoggers(time.Now().Add(1 * time.Hour))
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
