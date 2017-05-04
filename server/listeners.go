package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff"
)

const RETRY_AFTER_MIN = 5 * time.Second
const NOTEWORTHY_WAIT = 0 //1 * time.Minute
const RETRY_AFTER_MAX = 1 * time.Hour
const GIVE_UP_AFTER = 7 * 24 * time.Hour

var Listener_connections = int32(0)

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
		Log.Error("Giving up connectiong to %s (%s)", name, addr)
		return true
	} else if nb > NOTEWORTHY_WAIT {
		Log.Warning(err)
	}
	time.Sleep(nb)
	return false
}

func closeAndCheck(c io.Closer, name string) {
	err := c.Close()
	if err != nil {
		Log.Warning("error when closing %s: %s", name, err.Error())
	}
}

func readFile(path string, parser *PacketParser) {
	defer parser.Close()
	file, err := os.Open(path)
	Log.FatalIfErr(err, "open file")
	defer closeAndCheck(file, parser.SourceName)
	atomic.AddInt32(&Listener_connections, 1)
	reader := bufio.NewReaderSize(file, 512)
	lines := 0
	for {
		readStarted := time.Now()
		line, err := reader.ReadBytes(byte('\n'))
		lines += 1
		AisLog.Info("line %d", lines)
		parser.Accept(line, readStarted)
		if err != nil {
			if err != io.EOF {
				Log.Error("Error reading %s: %s",
					parser.SourceName, err.Error())
			}
			break
		}
	}
	atomic.AddInt32(&Listener_connections, -1)
	AisLog.FatalIf(Listener_connections == 0, "EOF")
}

func readTCP(addr string, silence_timeout time.Duration, parser *PacketParser) {
	defer parser.Close()
	b := newSourceBackoff()
	for {
		err := func() string { // scope for the defers
			addr, err := net.ResolveTCPAddr("tcp", addr)
			if err != nil {
				return fmt.Sprintf("Failed to resolve %ss adress (%s): %s",
					parser.SourceName, addr, err.Error())
			}
			conn, err := net.DialTCP("tcp", nil, addr)
			if err != nil {
				return fmt.Sprintf("Failed to connect to %s: %s",
					parser.SourceName, err.Error())
			}
			atomic.AddInt32(&Listener_connections, 1)
			defer atomic.AddInt32(&Listener_connections, -1)
			defer closeAndCheck(conn, parser.SourceName)
			// conn.CloseWrite() // causes EOFs from Kystverket
			buf := make([]byte, 4096)
			for {
				readStarted := time.Now()
				conn.SetReadDeadline(readStarted.Add(silence_timeout))
				n, err := conn.Read(buf)
				if err != nil {
					return fmt.Sprintf("%s read error: %s",
						parser.SourceName, err.Error())
				}
				parser.Accept(buf[:n], readStarted)
				b.Reset()
			}
		}()
		if handleSourceError(b, parser.SourceName, addr, err) {
			break
		}
	}
}

func readHTTP(url string, silence_timeout time.Duration, parser *PacketParser) {
	defer parser.Close()
	b := newSourceBackoff()
	// I think this modifies the global variable.
	// Trying to copy it results in a warning about copying mutexes,
	// and I don't know weither that's OK in this case.
	// The shortened timeout should be harmless
	transport := (http.DefaultTransport.(*http.Transport))
	transport.DialContext = newTimeoutConnDialer(silence_timeout)
	// net/http/httptrace doesn't seem to have anything for packets of body
	client := http.Client{
		Transport: transport,
		Jar:       nil,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 { // The default limit according to the documentation
				return http.ErrUseLastResponse
			}
			Log.Warning("%ss %s redirects to %s",
				parser.SourceName, via[0].URL, req.URL)
			return nil
		},
		Timeout: 0, // From start to close
	}
	for {
		err := func() string { // scope for the defers
			request, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return fmt.Sprintf("Failed to create request for %s: %s", url, err.Error())
			}
			resp, err := client.Do(request)
			if err != nil {
				return fmt.Sprintf("Failed to connect to %s: %s",
					parser.SourceName, err.Error())
			}
			atomic.AddInt32(&Listener_connections, 1)
			defer atomic.AddInt32(&Listener_connections, -1)
			defer closeAndCheck(resp.Body, parser.SourceName)
			// Body is only ReadCloser, and GzipReader isn't Conn so type asserting won't work.
			// If it did we could set its timeout directly
			// We could also check and branch to two different implementations.
			// if resp.Body.(net.Conn) != nil {
			// 	Log.Debug("http.Response.Body is a %T", resp.Body)
			// }
			// Can also try to http.Hijack it,
			// if I can force HTTP/1.1 and no compression thet could work.

			buf := make([]byte, 4096)
			for {
				readStarted := time.Now() // FIXME reuse time.Now() from timeoutConn.Read()?
				n, err := resp.Body.Read(buf)
				if err != nil {
					return fmt.Sprintf("%s read error: %s",
						parser.SourceName, err.Error())
				}
				parser.Accept(buf[:n], readStarted)
				b.Reset()
			}
		}()
		if handleSourceError(b, parser.SourceName, url, err) {
			break
		}
	}
}

func Read(name, url string, timeout time.Duration, merger *SourceMerger) *PacketParser {
	ph := NewPacketParser(name, Log, merger.Accept)
	if strings.HasPrefix(url, "http://") {
		go readHTTP(url, timeout, ph)
	} else if strings.HasPrefix(url, "tcp://") {
		go readTCP(url[len("tcp://"):], timeout, ph)
	} else if strings.HasPrefix(url, "file://") {
		go readFile(url[len("file://"):], ph)
	} else if strings.Contains(url, "://") {
		Log.Fatal("%s has unsupported protocol: %s", name, url)
	} else {
		go readFile(url, ph)
	}
	return ph
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
