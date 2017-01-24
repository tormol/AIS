package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	ais "github.com/andmarios/aislib"
)

var messageCount = 0

var (
	inputTCP = flag.String("inputTCP", "153.44.253.27:5631", "ip:port of tcp server to call")
)

func readAIS(c *net.TCPConn) {
	fmt.Println("Reading...")

	send := make(chan string, 1024*8)
	receive := make(chan ais.Message, 1024*8)
	failed := make(chan ais.FailedSentence, 1024*8)

	done := make(chan bool)

	go ais.Router(send, receive, failed)

	go func() {
		var message ais.Message
		var problematic ais.FailedSentence
		for {
			select {
			case message = <-receive:
				messageCount++
				switch message.Type {
				case 1, 2, 3:
					t, _ := ais.DecodeClassAPositionReport(message.Payload)
					fmt.Println(t.MMSI) //Printer kun ut MMSI foreløpig
				case 4:
					t, _ := ais.DecodeBaseStationReport(message.Payload)
					fmt.Println(t.MMSI)
				case 5:
					t, _ := ais.DecodeStaticVoyageData(message.Payload)
					fmt.Println(t.MMSI)
				case 8:
					t, _ := ais.DecodeBinaryBroadcast(message.Payload)
					fmt.Println(t.MMSI)
				case 18:
					t, _ := ais.DecodeClassBPositionReport(message.Payload)
					fmt.Println(t.MMSI)
				case 255:
					done <- true
				default:
					fmt.Printf("=== Message Type %2d ===\n", message.Type)
					fmt.Printf(" Unsupported type \n\n")
				}
			case problematic = <-failed:
				log.Println(problematic)
			}
		}
	}()

	for {
		buf := make([]byte, 1024)
		n, err := c.Read(buf)
		if err != nil {
			log.Println("Error: ", err)
			break
		}
		tcpMessage := string(buf[0:n])
		sentences := strings.Split(tcpMessage, "!")
		for i := 0; i < len(sentences); i++ {
			if sentences[i] != "" {
				send <- strings.TrimSpace(fmt.Sprintf("!" + sentences[i]))
			}
		}
	}
	close(send)
	<-done
}

func main() {
	flag.Parse()
	fmt.Println("Starting up...")

	addr, err := net.ResolveTCPAddr("tcp", *inputTCP)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	go readAIS(conn)
	time.Sleep(6 * time.Second)
	fmt.Printf("Messages decoded: %d\n", messageCount)
}
