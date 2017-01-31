package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

var out_connections int

/*
Forwards the TCP to either TCP or HTTP,
but sometimes pauses output to test timeout recovery.
Combine with Ctrl-C to test reconnects.
*/
func main() {
	send := make(chan []byte, 200)
	out_connections = 0
	listen := true
	ticker := time.NewTicker(8 * time.Second).C
	go func() {
		for _ = range ticker {
			listen = !listen
			fmt.Printf("out_connections: %d\n", out_connections)
			fmt.Printf("len(send): %d\n", len(send))
			// if listen == true {
			// 	//log.Println("starting")
			// 	go listen_tcp(send, &listen)
			// } else {
			// 	//log.Println("stopping")
			// 	go func() {
			// 		for {
			// 			select {
			// 			case _ = <-send: // flush
			// 			default:
			// 				return
			// 			}
			// 		}
			// 	}()
			// }
		}
	}()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "HTTP":
			fallthrough
		case "http":
			go serve_http(send)
		case "TCP":
			fallthrough
		case "tcp":
			go serve_tcp(send)
		default:
			log.Fatalln("Invalid protocol, must be http or tcp")
		}
	}
	go listen_tcp(send, &listen)
	time.Sleep(time.Hour)
}

func listen_tcp(send chan []byte, listen *bool) {
	addr, err := net.ResolveTCPAddr("tcp", "153.44.253.27:5631")
	CheckErr(err, "Resolve tcp address")
	conn, err := net.DialTCP("tcp", nil, addr)
	CheckErr(err, "listen to tcp")
	defer conn.Close() // FIXME can fail
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		CheckErr(err, "read tcp")
		if *listen && len(send) < cap(send) {
			send <- (buf[0:n])
			fmt.Println(string(buf[0:n]))
		}
	}
}

func serve_http(send chan []byte) {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		out_connections++
		defer func() { out_connections-- }()
		//w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Server", "test_timeout")
		w.WriteHeader(http.StatusOK)
		//go func() {
		for s := range send {
			_, err := w.Write(s)
			if err != nil {
				fmt.Printf("write to HTTP error: %s\n", err)
				return
			}
			w.(http.Flusher).Flush()
		}
		//}()
	})
	err := http.ListenAndServe("127.0.0.1:12345", nil)
	CheckErr(err, "listen to HTTP")
}

func serve_tcp(send chan []byte) {
	a, err := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")
	CheckErr(err, "resolve kystverket address")
	l, err := net.ListenTCP("tcp", a)
	CheckErr(err, "listen to TCP")
	defer l.Close()
	for {
		c, err := l.AcceptTCP()
		CheckErr(err, "accept TCP connection")
		go func() {
			defer c.Close()
			out_connections++
			defer func() { out_connections-- }()
			for s := range send {
				_, err := c.Write(s)
				if err != nil {
					fmt.Printf("write to TCP error: %s\n", err)
					break
				}
			}
		}()
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
