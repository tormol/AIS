package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
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

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			Log.Fatal(err.Error())
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
	go HttpServer("localhost:8080", newForwarder, a)
	go ForwardRawTCPServer("localhost:8023", newForwarder) // telnet port
	go ForwardRawUDPServer("localhost:8023", newForwarder)

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

	Read("ECC", "http://aishub.ais.ecc.no/raw", 5*time.Second, sm)
	Read("kystverket", "tcp://153.44.253.27:5631", 5*time.Second, sm)
	//Read("http_timeout", "http://127.0.0.1:12340", 8*time.Second, sm)
	//Read("tcp_timeout", "tcp://127.0.0.1:12341", 2*time.Second, sm)
	//Read("redirect", "http://localhost:12342", 0*time.Second, sm)
	//Read("redirect_loop", "http://localhost:12343", 0*time.Second, sm)
	//Read("http_flood", "http://localhost:12344", 2*time.Second, sm)
	//Read("tcp_flood", "tcp://localhost:12345", 2*time.Second, sm)
	//Read("file", "minute_ecc.log", 0*time.Second, sm)

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
