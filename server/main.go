package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
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

type TimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *TimeoutConn) Read(buf []byte) (int, error) {
	c.SetReadDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(buf)
}
func NewTimeoutConnDialer(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, time.Second*5)
		tconn := TimeoutConn{
			Conn:    conn,
			timeout: timeout,
		}
		return &tconn, err
	}
}

type Packet struct {
	source   string
	received time.Time
	data     []byte
}

func main() {
	writer := make(chan Packet)
	send := make(chan string)
	logger := time.NewTicker(10 * time.Second).C
	go Log(logger)
	readAIS(send)
	go ReadHTTP("ECC", "http://aishub.ais.ecc.no/raw", 5*time.Second, writer)
	go ReadTCP("Kystverket", "153.44.253.27:5631", 5*time.Second, writer)
	//go ReadHTTP("test_timeout", "http://127.0.0.1:12345", 8*time.Second, writer)
	//go ReadTCP("test_timeout", "127.0.0.1:12345", 2*time.Second, writer)
	//go ReadHTTP("test_redirect", "http://localhost:12346", 0*time.Second, writer)
	//go ReadHTTP("test_redirect_loop", "http://localhost:12347", 0*time.Second, writer)
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

func Log(ticker <-chan time.Time) {
	for _ = range ticker {
		fmt.Println("source connections: ", connections)
	}
}

var connections = 0

func ReadTCP(name string, ip string, silence_timeout time.Duration, writer chan Packet) {
	for {
		func() { // scope for the defers
			connections++
			defer func() { connections-- }()
			addr, err := net.ResolveTCPAddr("tcp", ip)
			CheckErr(err, "Resolve tcp address")
			conn, err := net.DialTCP("tcp", nil, addr)
			CheckErr(err, "listen to tcp")
			defer conn.Close() // FIXME can fail
			//conn.CloseWrite()
			buf := make([]byte, 4096)
			for {
				conn.SetReadDeadline(time.Now().Add(silence_timeout))
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
		}()
	}
}

func ReadHTTP(name string, url string, silence_timeout time.Duration, writer chan Packet) {
	// I think this modifies the global variable.
	// Trying to copy it results in a warning about copying mutexes,
	// and I don't know weither that's OK in this case.
	// The shortened timeout should be harmless
	transport := (http.DefaultTransport.(*http.Transport))
	transport.DialContext = NewTimeoutConnDialer(silence_timeout)
	client := http.Client{
		Transport: transport,
		Jar:       nil,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 { // The default limit according to the documentation
				return http.ErrUseLastResponse
			}
			log.Printf("%ss %s redirects to %s\n", name, via[0].URL, req.URL)
			return nil
		},
		Timeout: 0, // From start to close
	}
	for {
		func() { // scope for the defers
			connections++
			defer func() { connections-- }()
			request, err := http.NewRequest("GET", url, nil)
			CheckErr(err, "Create request")
			resp, err := client.Do(request)
			CheckErr(err, fmt.Sprintf("connect to %ss receiver", name))
			defer resp.Body.Close()
			// Body is only ReadCloser, and GzipReader isn't Conn so type asserting won't work.
			// If it did we could set its timeout directly
			// We could also check and branch to two different implementations.
			// if resp.Body.(net.Conn) != nil {
			// 	fmt.Printf("http.Response.Body is a %T\n", resp.Body)
			// }

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
