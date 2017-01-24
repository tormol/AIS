package main

import (
	"fmt"
	//"io"
	"net/http"
	//"os"
	"log"
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
	resp, err := http.Get("http://aishub.ais.ecc.no/raw")
	CheckErr(err, "connect to eccs receiver")
	defer resp.Body.Close()
	fmt.Println(resp)
	buf := make([]byte, 4096)
	for i := 0; i < 10; i++ {
		n, err := resp.Body.Read(buf)

		CheckErr(err, "continue read")
		fmt.Printf("Packet with lenght %d:\n%s", n, string(buf[0:n]))
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
