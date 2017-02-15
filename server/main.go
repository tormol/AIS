package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Packet struct {
	source  string
	arrived time.Time
	content []byte
}

var (
	Log = NewLogger(os.Stderr, LOG_DEBUG, 10*time.Second)
	// For input sentence or message "errors"
	AisLog = NewLogger(os.Stdout, LOG_DEBUG, 0)
)

func main() {
	signalChan := make(chan os.Signal, 1)
	// Intercept ^C and `timeout`s.
	// Catching SIGPIPE has no effect if it was what Log wrote to that broke, as it's, well, broken.
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)

	Log.AddPeriodicLogger("source_connections", 20*time.Second, func(l *Logger, _ time.Duration) {
		l.Debug("source connections: %d", listener_connections)
	})
	packets := make(chan Packet, 200)
	send := make(chan string)
	readAIS(send)
	ecc := NewPacketHandler("ECC", packets)
	go ReadHTTP("http://aishub.ais.ecc.no/raw", 5*time.Second, ecc)
	kystverket := NewPacketHandler("Kystverket", packets)
	go ReadTCP("153.44.253.27:5631", 5*time.Second, kystverket)
	//test := NewPacketHandler("test", packets)
	//http.SourceName += "_timeout"
	//go ReadHTTP("http://127.0.0.1:12345", 8*time.Second, test)
	//tcp.SourceName += "_timeout"
	//go ReadTCP("test_timeout", "127.0.0.1:12345", 2*time.Second, test)
	//http.SourceName += " test_redirect"
	//go ReadHTTP("test_redirect", "http://localhost:12346", 0*time.Second, test)
	//loop := NewPacketHandler("test_redirect_loop", make(chan<- Packet))
	//go ReadHTTP("http://localhost:12347", 0*time.Second, loop)
	//test.SourceName = "file"
	//go ReadFile("minute_ecc.log", test)
	go func() {
		for packet := range packets {
			splitPacket(packet.content, send)
		}
	}()

	// Here we wait for CTRL-C or some other kill signal
	_ = <-signalChan
	Log.Info("\nPackets left: %d", len(packets))
	ecc.Log(Log)
	kystverket.Log(Log)
	//test.Log()
}
