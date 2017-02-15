package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const RETRY_AFTER_MIN = 5 * time.Second
const NOTEWORTHY_WAIT = 0 //1 * time.Minute
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

// Extract common code from listeners
type PacketHandler struct {
	SourceName    string
	started       time.Time
	totalReadTime time.Duration
	packets       uint
	bytes         uint
	dst           chan<- Packet
}

func NewPacketHandler(sourceName string, sendTo chan<- Packet) *PacketHandler {
	ph := &PacketHandler{
		started:    time.Now(),
		SourceName: sourceName,
		dst:        sendTo,
	}
	Log.AddPeriodicLogger(sourceName+"_packets", 40*time.Second, func(l *Logger, _ time.Duration) {
		ph.Log(l)
	})
	return ph
}
func (ph *PacketHandler) Close() {
	close(ph.dst)
	Log.RemovePeriodicLogger(ph.SourceName + "_packets")
}
func (ph *PacketHandler) Log(l *Logger) {
	// As numbers go up, errors due to incomplete updates should become insignificant.
	avg := 0 * time.Nanosecond
	if ph.packets != 0 {
		avg = time.Duration(ph.totalReadTime.Nanoseconds()/int64(ph.packets)) * time.Nanosecond
	}
	l.Info("%s: listened for %s, in channel: %d/%d, %d bytes, %d packets, avg read: %s",
		ph.SourceName, time.Now().Sub(ph.started), len(ph.dst), cap(ph.dst),
		ph.bytes, ph.packets, avg.String())
}

// bufferSlice cannot be sent to buffered channels: slicing doesn't copy.
func (ph *PacketHandler) accept(bufferSlice []byte, readStarted time.Time) {
	now := time.Now()
	ph.totalReadTime += now.Sub(readStarted)
	ph.packets++
	ph.bytes += uint(len(bufferSlice))

	//AisLog.Debug(Escape(bufferSlice))
	//AisLog.Debug("%dB: %s", len(bufferSlice), Escape(bufferSlice))
	content := make([]byte, len(bufferSlice))
	copy(content, bufferSlice)
	if len(ph.dst) == cap(ph.dst) {
		Log.Warning("%s channel full", ph.SourceName)
	}
	ph.dst <- Packet{
		source:  ph.SourceName,
		arrived: now,
		content: content,
	}
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

func ReadFile(path string, handler *PacketHandler) {
	defer handler.Close()
	file, err := os.Open(path)
	Log.FatalIfErr(err, "open file")
	defer file.Close()
	reader := bufio.NewReaderSize(file, 512)
	lines := 0
	for {
		readStarted := time.Now()
		line, err := reader.ReadBytes(byte('\n'))
		lines += 1
		Log.Debug("line %d", lines)
		handler.accept(line, readStarted)
		if err != nil {
			if err != io.EOF {
				Log.Error("Error reading %s: %s",
					handler.SourceName, err.Error())
			}
			break
		}
	}
}

func ReadTCP(addr string, silence_timeout time.Duration, handler *PacketHandler) {
	defer handler.Close()
	b := newSourceBackoff()
	for {
		err := func() string { // scope for the defers
			listener_connections++
			defer func() { listener_connections-- }()
			addr, err := net.ResolveTCPAddr("tcp", addr)
			if err != nil {
				return fmt.Sprintf("Failed to resolve %ss adress (%s): %s",
					handler.SourceName, addr, err.Error())
			}
			conn, err := net.DialTCP("tcp", nil, addr)
			if err != nil {
				return fmt.Sprintf("Failed to connect to %s: %s",
					handler.SourceName, err.Error())
			}
			defer conn.Close() // FIXME can fail
			//conn.CloseWrite()
			buf := make([]byte, 4096)
			for {
				readStarted := time.Now()
				conn.SetReadDeadline(readStarted.Add(silence_timeout))
				n, err := conn.Read(buf)
				if err != nil {
					return fmt.Sprintf("%s read error: %s",
						handler.SourceName, err.Error())
				}
				handler.accept(buf[:n], readStarted)
				b.Reset()
			}
		}()
		if handleSourceError(b, handler.SourceName, addr, err) {
			break
		}
	}
}

func ReadHTTP(url string, silence_timeout time.Duration, handler *PacketHandler) {
	defer handler.Close()
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
				handler.SourceName, via[0].URL, req.URL)
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
				return fmt.Sprintf("Failed to connect to %s: %s",
					handler.SourceName, err.Error())
			}
			defer resp.Body.Close()
			// Body is only ReadCloser, and GzipReader isn't Conn so type asserting won't work.
			// If it did we could set its timeout directly
			// We could also check and branch to two different implementations.
			// if resp.Body.(net.Conn) != nil {
			// 	Log.Debug("http.Response.Body is a %T", resp.Body)
			// }

			buf := make([]byte, 4096)
			for {
				readStarted := time.Now() // FIXME reuse time.Now() from timeoutConn.Read()?
				n, err := resp.Body.Read(buf)
				if err != nil {
					return fmt.Sprintf("%s read error: %s",
						handler.SourceName, err.Error())
				}
				handler.accept(buf[:n], readStarted)
				b.Reset()
			}
		}()
		if handleSourceError(b, handler.SourceName, url, err) {
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
