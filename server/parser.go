package main

import (
	"fmt"
	"strings"

	ais "github.com/andmarios/aislib"
)

var messageCount = 0

func splitPacket(packet []byte, send chan string) {
	sentences := strings.Split(string(packet), "!")
	for i := 0; i < len(sentences); i++ {
		if sentences[i] != "" {
			send <- strings.TrimSpace(fmt.Sprint("!" + sentences[i]))
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
					AisLog.Debug("%09d", t.MMSI) //Printer kun ut MMSI foreløpig
				case 4:
					t, _ := ais.DecodeBaseStationReport(message.Payload)
					AisLog.Debug("%09d", t.MMSI)
				case 5:
					t, _ := ais.DecodeStaticVoyageData(message.Payload)
					AisLog.Debug("%09d", t.MMSI)
				case 8:
					t, _ := ais.DecodeBinaryBroadcast(message.Payload)
					AisLog.Debug("%09d", t.MMSI)
				case 18:
					t, _ := ais.DecodeClassBPositionReport(message.Payload)
					AisLog.Debug("%09d", t.MMSI)
				case 255:
					done <- true
				default:
					AisLog.Debug("Unsupported message type %2d", message.Type)
				}
			case problematic = <-failed:
				AisLog.Debug("%s\n%s", problematic.Sentence, problematic.Issue)
			}
		}
	}()
	//<-done
}
