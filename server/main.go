package main

import (
	"fmt"
	"log"
	"time"
)

type Packet struct {
	source  string
	arrived time.Time
	content []byte
}

func main() {
	writer := make(chan Packet)
	send := make(chan string)
	logger := time.NewTicker(10 * time.Second).C
	go Log(logger)
	readAIS(send)
	go ReadHTTP("ECC", "http://aishub.ais.ecc.no/raw", 5*time.Second, writer)
	go ReadTCP("Kystverket", "153.44.253.27:5631", 5*time.Second, writer)
	//go ReadHTTP("test_timeout", "http://127.0.0.1:12345", 8*time.Second, writer)
	//go ReadTCP("test_timeout", "127.0.0.1:12345", 2*time.Second, writer)
	//go ReadHTTP("test_redirect", "http://localhost:12346", 0*time.Second, writer)
	//go ReadHTTP("test_redirect_loop", "http://localhost:12347", 0*time.Second, writer)
	for packet := range writer {
		splitPacket(packet.content, send)
		//line := string(packet.data) // TODO split just in case
		//fmt.Printf("Packet with lenght %d from %s:\n%s", len(line), packet.source, line)
		if len(writer) == 20 {
			fmt.Printf("channel has backlog")
		}
	}
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
