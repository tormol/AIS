package main

import (
	"fmt"
	"log"
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

func main() {
	signalChan := make(chan os.Signal, 1)
	// Intercept ^C and `timeout`s
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	packets := make(chan Packet, 200) // share until we start assembling multi-sentcence messages
	send := make(chan string)
	logger := time.NewTicker(10 * time.Second).C
	go Log(logger)
	readAIS(send)
	ecc := NewPacketHandler("ECC", packets)
	go ReadHTTP("http://aishub.ais.ecc.no/raw", 5*time.Second, &ecc)
	kystverket := NewPacketHandler("Kystverket", packets)
	go ReadTCP("153.44.253.27:5631", 5*time.Second, &kystverket)
	//test := NewPacketHandler("test", packets)
	//http.SourceName += "_timeout"
	//go ReadHTTP("http://127.0.0.1:12345", 8*time.Second, &test)
	//tcp.SourceName += "_timeout"
	//go ReadTCP("test_timeout", "127.0.0.1:12345", 2*time.Second, &test)
	//http.SourceName += " test_redirect"
	//go ReadHTTP("test_redirect", "http://localhost:12346", 0*time.Second, &test)
	//loop := NewPacketHandler("test_redirect_loop", make(chan<- Packet))
	//go ReadHTTP("http://localhost:12347", 0*time.Second, &loop)
	//test.SourceName = "file"
	//go ReadFile("minute_ecc.log", &test)
	go func() {
		for packet := range packets {
			splitPacket(packet.content, send)
		}
	}()

	// Here we wait for CTRL-C or some other kill signal
	_ = <-signalChan
	log.Println("Packets left: ", len(packets))
	log.Println(ecc.Log())
	log.Println(kystverket.Log())
	//log.Println(test.Log())
}

func Log(ticker <-chan time.Time) {
	for _ = range ticker {
		fmt.Println("source connections: ", listener_connections)
	}
}

func CheckErr(err error, msg string) {
	if err != nil {
		log.Fatalf("Failed to %s: %s\n", msg, err.Error())
	}
}
func ErrIf(cond bool, msg string) {
	if cond {
		log.Fatalln(msg)
	}
}
