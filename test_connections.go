package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

var outConnections int

/*
Forwards the TCP to either TCP or HTTP,
but sometimes pauses output to test timeout recovery.
Combine with Ctrl-C to test reconnects.

Test all at once with go run server/*go -- http_timeout:8s=http://localhost:12340 tcp_timeout:2s=tcp://localhost:12341 redirect:0=http://localhost:12342 redirect_loop:0=http://localhost:12343 http_flood:2s=http://localhost:12344 tcp_flood:2s=tcp://localhost:12345
*/
func main() {
	outConnections = 0
	notPaused := true
	ticker := time.NewTicker(8 * time.Second).C
	go func() {
		for range ticker {
			notPaused = !notPaused
			fmt.Printf("outConnections: %d\n", outConnections)
		}
	}()

	go timeoutHTTP(&notPaused)
	go timeoutTCP(&notPaused)
	go redirectOnce()
	go redirectLoop()
	go floodHTTP()
	go floodTCP()
	time.Sleep(time.Hour)
}

func readPauseTCP(send chan<- []byte, notPaused, stop *bool) {
	addr, err := net.ResolveTCPAddr("tcp", "153.44.253.27:5631")
	CheckErr(err, "Resolve kystverket address")
	conn, err := net.DialTCP("tcp", nil, addr)
	CheckErr(err, "Connect to kystverket")
	defer conn.Close() // FIXME can fail
	buf := make([]byte, 4096)
	for !*stop {
		n, err := conn.Read(buf)
		CheckErr(err, "read tcp")
		if *notPaused && len(send) < cap(send) {
			content := make([]byte, n)
			copy(content, buf[:n])
			send <- content
			fmt.Println(string(buf[0:n]))
		}
	}
}

// Connect to with http_timeout:8s=http://localhost:12340
func timeoutHTTP(notPaused *bool) {
	read := make(chan []byte, 200)
	h := func(w http.ResponseWriter, _ *http.Request) {
		// I guess the caller closes the connection...
		outConnections++
		defer func() { outConnections-- }()
		stop := false
		defer func() { stop = true }()
		go readPauseTCP(read, notPaused, &stop)
		//w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Server", "test_connections")
		w.WriteHeader(http.StatusOK)
		//go func() {
		for s := range read {
			_, err := w.Write(s)
			if err != nil {
				fmt.Printf("write to HTTP error: %s\n", err)
				return
			}
			w.(http.Flusher).Flush()
		}
		//}()
	}
	s := http.Server{
		Addr:    "localhost:12340",
		Handler: http.HandlerFunc(h),
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

// Connect to with tcp_timeout:2s=tcp://localhost:12341
func timeoutTCP(notPaused *bool) {
	a, err := net.ResolveTCPAddr("tcp", "localhost:12341")
	CheckErr(err, "resolve TCP address")
	l, err := net.ListenTCP("tcp", a)
	CheckErr(err, "listen for TCP")
	defer closeAndCheck(l, "timeoutTCP server")
	read := make(chan []byte, 200)
	for {
		c, err := l.AcceptTCP()
		CheckErr(err, "accept TCP connection")
		go func() {
			defer closeAndCheck(c, "timeoutTCP connection")
			outConnections++
			defer func() { outConnections-- }()
			stop := false
			defer func() { stop = true }()
			go readPauseTCP(read, notPaused, &stop)
			for s := range read {
				_, err := c.Write(s)
				if err != nil {
					fmt.Printf("write to TCP error: %s\n", err)
					break
				}
			}
		}()
	}
}

// Connect to with redirect:2s=http://localhost:12342
func redirectOnce() {
	h := http.RedirectHandler("http://localhost:12340", http.StatusMovedPermanently)
	s := http.Server{
		Addr:    "localhost:12342",
		Handler: h,
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

// Connect to with redirect_loop:0=http://localhost:12343
func redirectLoop() {
	h := http.RedirectHandler("http://localhost:12343", http.StatusMovedPermanently)
	s := http.Server{
		Addr:    "localhost:12343",
		Handler: h,
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

const floodPacket = "!BSVDM,2,1,6,A,59NSF?02;Ic4DiPoP00i0Nt>0t@E8L5<0000001@:H@964Q60;lPASQDh000,0*11\r\n!BSVDM,2,2,6,A,00000000000,2*3B\r\n"

// Connect to with http_flood:2s=http://localhost:12344
func floodHTTP() {
	h := func(w http.ResponseWriter, _ *http.Request) {
		// I guess the caller closes the connection...
		outConnections++
		defer func() { outConnections-- }()
		//w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Server", "test_connections")
		w.WriteHeader(http.StatusOK)
		//go func() {
		for {
			_, err := w.Write([]byte(floodPacket))
			if err != nil {
				fmt.Printf("write to HTTP error: %s\n", err)
				return
			}
			w.(http.Flusher).Flush()
		}
		//}()
	}
	s := http.Server{
		Addr:    "localhost:12344",
		Handler: http.HandlerFunc(h),
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

// Connect to with tcp_flood:2s=tcp://localhost:12345
func floodTCP() {
	a, err := net.ResolveTCPAddr("tcp", "localhost:12345")
	CheckErr(err, "resolve TCP address")
	l, err := net.ListenTCP("tcp", a)
	CheckErr(err, "listen for TCP")
	defer closeAndCheck(l, "floodTCP server")
	for {
		c, err := l.AcceptTCP()
		CheckErr(err, "accept TCP connection")
		go func() {
			outConnections++
			defer closeAndCheck(c, "floodTcp connection")
			defer func() { outConnections-- }()
			for {
				_, err := c.Write([]byte(floodPacket))
				if err != nil {
					fmt.Printf("write to TCP error: %s\n", err)
					break
				}
			}
		}()
	}
}

func closeAndCheck(c io.Closer, name string) {
	err := c.Close()
	if err != nil {
		log.Fatalf("error when closing %s: %s", name, err.Error())
	}
}

// CheckErr aborts the process if err is not nil
func CheckErr(err error, msg string) {
	if err != nil {
		log.Fatalf("Failed to %s: %s\n", msg, err.Error())
	}
}
