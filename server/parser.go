package main

import (
	"fmt"
	"log"
	"strings"
	//	"time"

	ais "github.com/andmarios/aislib"
)

var messageCount = 0

func splitPacket(packet []byte, send chan string) {
	sentences := strings.Split(string(packet), "!")
	for i := 0; i < len(sentences); i++ {
		if sentences[i] != "" {
			send <- strings.TrimSpace(fmt.Sprintf("!" + sentences[i]))
		}
	}
}

func readAIS(send chan string) {
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
	//<-done
}
