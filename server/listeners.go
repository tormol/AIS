package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

const RETRY_AFTER_MIN = 5 * time.Second
const NOTEWORTHY_WAIT = 1 * time.Minute
const RETRY_AFTER_MAX = 1 * time.Hour
const GIVE_UP_AFTER = 7 * 24 * time.Hour

var listener_connections = 0

func newSourceBackoff() *backoff.ExponentialBackOff {
	eb := backoff.NewExponentialBackOff()
	eb.InitialInterval = RETRY_AFTER_MIN
	eb.MaxInterval = RETRY_AFTER_MAX
	eb.MaxElapsedTime = GIVE_UP_AFTER
	eb.Reset() // current interval
	return eb
}

func handleSourceError(b *backoff.ExponentialBackOff, name, addr, err string) bool {
	nb := b.NextBackOff()
	if nb == backoff.Stop {
		log.Printf("Giving up connectiong to %s (%s)\n", name, addr)
		return true
	} else if nb > NOTEWORTHY_WAIT {
		log.Println(err)
	}
	time.Sleep(nb)
	return false
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
	b := newSourceBackoff()
	for {
		err := func() string { // scope for the defers
			listener_connections++
			defer func() { listener_connections-- }()
			addr, err := net.ResolveTCPAddr("tcp", addr)
			if err != nil {
				return fmt.Sprintf("Failed to resolve %ss adress (%s): %s",
					name, addr, err.Error())
			}
			conn, err := net.DialTCP("tcp", nil, addr)
			if err != nil {
				return fmt.Sprintf("Failed to connect to %s: %s", name, err.Error())
			}
			defer conn.Close() // FIXME can fail
			//conn.CloseWrite()
			buf := make([]byte, 4096)
			for {
				conn.SetReadDeadline(time.Now().Add(silence_timeout))
				n, err := conn.Read(buf)
				if err != nil {
					return fmt.Sprintf("%s read error: %s", name, err.Error())
				}
				writer <- Packet{
					source:  name,
					arrived: time.Now(),
					content: buf[:n],
				}
				b.Reset()
			}
		}()
		if handleSourceError(b, name, addr, err) {
			break
		}
	}
}

func ReadHTTP(name string, url string, silence_timeout time.Duration, writer chan<- Packet) {
	b := newSourceBackoff()
	// I think this modifies the global variable.
	// Trying to copy it results in a warning about copying mutexes,
	// and I don't know weither that's OK in this case.
	// The shortened timeout should be harmless
	transport := (http.DefaultTransport.(*http.Transport))
	transport.DialContext = newTimeoutConnDialer(silence_timeout)
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
		err := func() string { // scope for the defers
			listener_connections++
			defer func() { listener_connections-- }()
			request, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return fmt.Sprintf("Failed to create request for %s: %s", url, err.Error())
			}
			resp, err := client.Do(request)
			if err != nil {
				return fmt.Sprintf("Failed to connect to %s: %s", name, err.Error())
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
					return fmt.Sprintf("%s read error: %s", name, err.Error())
				}
				writer <- Packet{
					source:  name,
					arrived: time.Now(),
					content: buf[:n],
				}
				b.Reset()
			}
		}()
		if handleSourceError(b, name, url, err) {
			break
		}
	}
}

// Adapted from https://gist.github.com/jbardin/9663312
type timeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *timeoutConn) Read(buf []byte) (int, error) {
	c.SetReadDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(buf)
}
func newTimeoutConnDialer(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, time.Second*5)
		tconn := timeoutConn{
			Conn:    conn,
			timeout: timeout,
		}
		return &tconn, err
	}
}
