package main

import (
	"bufio"
	"context"
	"github.com/cenkalti/backoff"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

var listener_connections = 0

func NewSourceBackoff() *backoff.ExponentialBackOff {
	eb := backoff.NewExponentialBackOff()
	eb.InitialInterval = 5 * time.Second
	eb.MaxInterval = 1 * time.Hour
	eb.MaxElapsedTime = 7 * 24 * time.Hour
	eb.Reset() // current interval
	return eb
}

func ReadFile(path string, writer chan<- Packet) {
	file, err := os.Open(path)
	CheckErr(err, "open file")
	defer file.Close()
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line += 1
		log.Printf("line %d\n", line)
		writer <- Packet{
			source:  path,
			arrived: time.Now(),
			content: []byte(scanner.Text()),
		}
	}
	CheckErr(scanner.Err(), "read from file")
}

func ReadTCP(name string, addr string, silence_timeout time.Duration, writer chan<- Packet) {
	b := NewSourceBackoff()
	for {
		func() { // scope for the defers
			listener_connections++
			defer func() { listener_connections-- }()
			addr, err := net.ResolveTCPAddr("tcp", addr)
			if err != nil {
				log.Printf("Failed to resolve %ss adress (%s): %s\n",
					name, addr, err.Error())
				return
			}
			conn, err := net.DialTCP("tcp", nil, addr)
			if err != nil {
				log.Printf("Failed to connect to %s: %s\n", name, err.Error())
				return
			}
			defer conn.Close() // FIXME can fail
			//conn.CloseWrite()
			buf := make([]byte, 4096)
			for {
				conn.SetReadDeadline(time.Now().Add(silence_timeout))
				n, err := conn.Read(buf)
				if err != nil {
					log.Printf("%s read error: %s\n", name, err.Error())
					break
				} else {
					writer <- Packet{
						source:  name,
						arrived: time.Now(),
						content: buf[0:n],
					}
					b.Reset()
				}
			}
		}()
		nb := b.NextBackOff()
		if nb == backoff.Stop {
			log.Printf("Giving up connectiong to %s (%s)\n", name, addr)
			break
		}
		time.Sleep(nb)
	}
}

func ReadHTTP(name string, url string, silence_timeout time.Duration, writer chan<- Packet) {
	b := NewSourceBackoff()
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
			listener_connections++
			defer func() { listener_connections-- }()
			request, err := http.NewRequest("GET", url, nil)
			if err != nil {
				log.Printf("Failed to create request for %s: %s\n", url, err.Error())
				return
			}
			resp, err := client.Do(request)
			if err != nil {
				log.Printf("Failed to connect to %s: %s\n", name, err.Error())
				return
			}
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
					log.Printf("%s read error: %s\n", name, err.Error())
					break
				} else {
					writer <- Packet{
						source:  name,
						arrived: time.Now(),
						content: buf[0:n],
					}
					b.Reset()
				}
			}
		}()
		nb := b.NextBackOff()
		if nb == backoff.Stop {
			log.Printf("Giving up connectiong to %s (%s)\n", name, url)
			break
		}
		time.Sleep(nb)
	}
}

type timeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *timeoutConn) Read(buf []byte) (int, error) {
	c.SetReadDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(buf)
}
func NewTimeoutConnDialer(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, time.Second*5)
		tconn := timeoutConn{
			Conn:    conn,
			timeout: timeout,
		}
		return &tconn, err
	}
}
