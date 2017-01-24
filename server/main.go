package main

import (
	"fmt"
	//"io"
	"net/http"
	//"os"
	"log"
	"time"
)

type Listener interface {
	LastReceived() uint64 // timestamp

}
type Ship struct {
}

// three sources
// scaling: if parsing takes longer than dead time between messages, need to send to group
//

type Packet struct {
	source string
	data   []byte
}

func main() {
	writer := make(chan Packet)
	go ReadHttp("ecc", "http://aishub.ais.ecc.no/raw", writer)
	for packet := range writer {
		line := string(packet.data) // TODO split just in case
		fmt.Printf("Packet with lenght %d from %s:\n%s", len(line), packet.source, line)
	}
}

func ReadHttp(name string, url string, writer chan Packet) {
	client := http.Client{
		Transport:     nil,
		Jar:           nil,
		CheckRedirect: nil, // TODO log
		Timeout:       0,   // TODO test
	}
	for {
		request, err := http.NewRequest("GET", url, nil)
		CheckErr(err, "Create request")
		stopper := make(chan struct{})
		go stopAfter(2, stopper)
		request.Cancel = stopper
		resp, err := client.Do(request)
		CheckErr(err, "connect to eccs receiver")
		defer resp.Body.Close()
		fmt.Println(resp)
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if err != nil {
				log.Printf("\n\n\nread error: %s\n", err.Error())
				break
			} else {
				writer <- Packet{
					source: name,
					data:   buf[0:n],
				}
			}
		}
	}
}

func stopAfter(seconds int, stopper chan struct{}) {
	tickCh := time.Tick(time.Duration(seconds) * time.Second)
	_ = <-tickCh
	stopper <- struct{}{}
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
