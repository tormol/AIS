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

func main() {
	client := http.Client{
		Transport:     nil,
		Jar:           nil,
		CheckRedirect: nil, // TODO log
		Timeout:       0,   //TODO test
	}
	request, err := http.NewRequest("GET", "http://aishub.ais.ecc.no/raw", nil)
	CheckErr(err, "Create request")
	stopper := make(chan struct{})
	go stopAfter(5, stopper)
	request.Cancel = stopper
	resp, err := client.Do(request)
	CheckErr(err, "connect to eccs receiver")
	defer resp.Body.Close()
	fmt.Println(resp)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		CheckErr(err, "continue read")
		fmt.Printf("Packet with lenght %d:\n%s", n, string(buf[0:n]))
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
