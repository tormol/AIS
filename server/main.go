package main

import (
	"fmt"
	//"io"
	"net"
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
	source   string
	received time.Time
	data     []byte
}

func main() {
	writer := make(chan Packet)
	send := make(chan string)
	readAIS(send)
	go ReadHttp("ECC", "http://aishub.ais.ecc.no/raw", writer)
	go ReadTCP("Kystverket", "153.44.253.27:5631", writer)
	for packet := range writer {
		splitPacket(packet.data, send)
		//line := string(packet.data) // TODO split just in case
		//fmt.Printf("Packet with lenght %d from %s:\n%s", len(line), packet.source, line)
		if len(writer) == 20 {
			fmt.Printf("channel has backlog")
		}
	}
}

func WatchBuffer(buffer chan Packet) {

}

func ReadTCP(name string, ip string, writer chan Packet) {
	for {
		addr, err := net.ResolveTCPAddr("tcp", ip)
		CheckErr(err, "Resolve tcp address")
		conn, err := net.DialTCP("tcp", nil, addr)
		CheckErr(err, "listen to tcp")
		defer conn.Close() // FIXME can fail
		//conn.CloseWrite()
		buf := make([]byte, 4096)
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				log.Printf("\n\n\n%s read error: %s\n", name, err.Error())
				break
			} else {
				writer <- Packet{
					source:   name,
					received: time.Now(),
					data:     buf[0:n],
				}
			}
		}
	}
}

func ReadHttp(name string, url string, writer chan Packet) {
	client := http.Client{
		Transport:     nil,
		Jar:           nil,
		CheckRedirect: nil, // TODO log
		Timeout:       0,   // From start of connection
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
				log.Printf("\n\n\n%s read error: %s\n", name, err.Error())
				break
			} else {
				writer <- Packet{
					source:   name,
					received: time.Now(),
					data:     buf[0:n],
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
