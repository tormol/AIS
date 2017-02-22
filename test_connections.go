package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

var out_connections int

/*
Forwards the TCP to either TCP or HTTP,
but sometimes pauses output to test timeout recovery.
Combine with Ctrl-C to test reconnects.
*/
func main() {
	out_connections = 0
	not_paused := true
	ticker := time.NewTicker(8 * time.Second).C
	go func() {
		for _ = range ticker {
			not_paused = !not_paused
			fmt.Printf("out_connections: %d\n", out_connections)
		}
	}()

	go Timeout_HTTP(&not_paused)
	go Timeout_TCP(&not_paused)
	go Redirect_once()
	go Redirect_loop()
	go Flood_HTTP()
	go Flood_TCP()
	time.Sleep(time.Hour)
}

func read_pause_TCP(send chan<- []byte, not_paused, stop *bool) {
	addr, err := net.ResolveTCPAddr("tcp", "153.44.253.27:5631")
	CheckErr(err, "Resolve kystverket address")
	conn, err := net.DialTCP("tcp", nil, addr)
	CheckErr(err, "Connect to kystverket")
	defer conn.Close() // FIXME can fail
	buf := make([]byte, 4096)
	for !*stop {
		n, err := conn.Read(buf)
		CheckErr(err, "read tcp")
		if *not_paused && len(send) < cap(send) {
			content := make([]byte, n)
			copy(content, buf[:n])
			send <- content
			fmt.Println(string(buf[0:n]))
		}
	}
}

func Timeout_HTTP(not_paused *bool) {
	read := make(chan []byte, 200)
	h := func(w http.ResponseWriter, _ *http.Request) {
		// I guess the caller closes the connection...
		out_connections++
		defer func() { out_connections-- }()
		stop := false
		defer func() { stop = true }()
		go read_pause_TCP(read, not_paused, &stop)
		//w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Server", "test_timeout")
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
		Addr:    "127.0.0.1:12340",
		Handler: http.HandlerFunc(h),
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

func Timeout_TCP(not_paused *bool) {
	a, err := net.ResolveTCPAddr("tcp", "127.0.0.1:12341")
	CheckErr(err, "resolve TCP address")
	l, err := net.ListenTCP("tcp", a)
	CheckErr(err, "listen for TCP")
	defer closeAndCheck(l, "timeout_TCP server")
	read := make(chan []byte, 200)
	for {
		c, err := l.AcceptTCP()
		CheckErr(err, "accept TCP connection")
		go func() {
			defer closeAndCheck(c, "timeout_TCP connection")
			out_connections++
			defer func() { out_connections-- }()
			stop := false
			defer func() { stop = true }()
			go read_pause_TCP(read, not_paused, &stop)
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

func Redirect_once() {
	h := http.RedirectHandler("http://127.0.0.1:12340", http.StatusMovedPermanently)
	s := http.Server{
		Addr:    "127.0.0.1:12342",
		Handler: h,
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

func Redirect_loop() {
	h := http.RedirectHandler("http://127.0.0.1:12343", http.StatusMovedPermanently)
	s := http.Server{
		Addr:    "127.0.0.1:12343",
		Handler: h,
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

const floodPacket = "!BSVDM,2,1,6,A,59NSF?02;Ic4DiPoP00i0Nt>0t@E8L5<0000001@:H@964Q60;lPASQDh000,0*11\r\n!BSVDM,2,2,6,A,00000000000,2*3B\r\n"

func Flood_HTTP() {
	h := func(w http.ResponseWriter, _ *http.Request) {
		// I guess the caller closes the connection...
		out_connections++
		defer func() { out_connections-- }()
		//w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Server", "test_timeout")
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
		Addr:    "127.0.0.1:12344",
		Handler: http.HandlerFunc(h),
	}
	err := s.ListenAndServe()
	CheckErr(err, "listen to HTTP")
}

func Flood_TCP() {
	a, err := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")
	CheckErr(err, "resolve TCP address")
	l, err := net.ListenTCP("tcp", a)
	CheckErr(err, "listen for TCP")
	defer closeAndCheck(l, "flood_tcp server")
	for {
		c, err := l.AcceptTCP()
		CheckErr(err, "accept TCP connection")
		go func() {
			out_connections++
			defer closeAndCheck(c, "flood_tcp connection")
			defer func() { out_connections-- }()
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
