package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	FORWARDER_CHANNEL_CAP = 20
	CLOSE_FORWARDER_AFTER = 20 /* messages dropped in a row */
	UDP_FORWARDER_TIMEOUT = 5 * time.Second
)

// Abstracts the actual trait away from other files
type NewForwarder interface {
	io.WriteCloser
}

// A WriteCloser for http forwarding
type httpForwarderConn struct {
	http.ResponseWriter // implements io.Writer
	// Request details doesn't matter any longer
	ended chan struct{} // For the request handles to block on
}

func (hfc *httpForwarderConn) Close() error {
	hfc.ended <- struct{}{} // makes handler return
	return nil              // the Responsewriter is closed when the handler returns
}

// Sets up the writer for forwarding and passes it to add.
// Doesn't eturns until the client disconnects or there is an I/O error.
func ForwardRawHTTP(add chan<- NewForwarder, w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
	w.Header().Set("Transfer-Encoding", "chunked")
	// Need to stay in this function while the connection lasts,
	// so there is no point in trying to extract (Hijack) a TCPConn.
	w.WriteHeader(http.StatusOK)
	hfc := &httpForwarderConn{w, make(chan struct{})}
	add <- hfc
	<-hfc.ended
}

// Listens for TCP requests and passes the connections to add.
// Never returns.
func ForwardRawTCPServer(serveAddr string, add chan<- NewForwarder) {
	a, err := net.ResolveTCPAddr("tcp", serveAddr)
	Log.FatalIfErr(err, "resolve raw forwarding TCP address")
	l, err := net.ListenTCP("tcp", a)
	Log.FatalIfErr(err, "listen for TCP")
	defer l.Close()
	for {
		conn, err := l.AcceptTCP()
		Log.FatalIfErr(err, "accept raw forwarding TCP connection")
		add <- conn // TCPConn implements WriteCloser
	}
}

// Because UDP is fire-and-forget, the client stopping listening won't cause an
// error, so we could easily end up sending forever.
// Therefore we need to time out after a while.
type udpForwarderConn struct {
	listener *net.UDPConn // immutable, used by forwarder
	to       *net.UDPAddr // immutable, used by forwarder
	flag     int32        // 0: running, 1: stop, 2: stopped
	timeout  time.Time    // not atomic; controlled by server
}

func (ufc *udpForwarderConn) Write(slice []byte) (int, error) {
	if atomic.CompareAndSwapInt32(&ufc.flag, 1, 2) {
		return 0, io.EOF
	}
	n, err := ufc.listener.WriteToUDP(slice, ufc.to)
	if err != nil {
		atomic.StoreInt32(&ufc.flag, 2)
	}
	return n, err
}
func (ufc *udpForwarderConn) Close() error {
	atomic.StoreInt32(&ufc.flag, 2)
	return nil
}

// Listen for UDP packets and start / stop / time out forwarders
// Never returns.
func ForwardRawUDPServer(listenAddr string, add chan<- NewForwarder) {
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	Log.FatalIfErr(err, "resolve raw forwarding UDP address")
	listener, err := net.ListenUDP("udp", laddr)
	Log.FatalIfErr(err, "listen for UDP")

	connections := make(map[string]*udpForwarderConn)
	stop := time.NewTicker(1 * time.Second).C
	start := make(chan *net.UDPAddr, 16)
	go func() { // Send packet source to channel so we can select over multiple.
		defer listener.Close()
		buf := make([]byte, 32)
		for {
			_, from, err := listener.ReadFromUDP(buf)
			Log.FatalIfErr(err, "accept raw forwarding UDP connection")
			start <- from
		}
	}()

	for {
		select {
		case from := <-start:
			now := time.Now()
			timeout := now.Add(UDP_FORWARDER_TIMEOUT)
			fromAddrStr := from.String()
			ufc := connections[fromAddrStr]
			if ufc == nil { // create new connection
				ufc = &udpForwarderConn{
					listener: listener,
					to:       from,
					flag:     0,
					timeout:  timeout,
				}
				connections[fromAddrStr] = ufc
				add <- ufc
			} else if atomic.LoadInt32(&ufc.flag) == 0 { // reset timeout if it hasn't been stopped
				ufc.timeout = timeout
			} else { // reset and restart if there somehow was an errror
				ufc.flag = 0
				ufc.timeout = timeout
				add <- ufc
			}
		case now := <-stop:
			// stop forwarding to clients we haven't heard anything from in
			for k, ufc := range connections {
				if now.After(ufc.timeout) {
					// only tell the forwarder to stop if it's running
					atomic.CompareAndSwapInt32(&ufc.flag, 0, 1)
					delete(connections, k)
				}
			}
		}
	}
}

// Info about forwarders stored by ForwarderManager()
type forwardConn struct {
	send    chan<- []byte
	fullFor int64
}

// Starts new forwarders and cancels them if they stop consuming messages.
// Never returns.
func ForwarderManager(messages <-chan *Message, add <-chan NewForwarder) {
	prevToken := int64(0) // monotonically increasing ID sent when a forwarder stops on its own.
	connections := make(map[int64]forwardConn)
	closer := make(chan int64) // unbuffered
	for {
		select {
		case m := <-messages: // new message to forward
			text := m.Sentences[0].Text
			if len(m.Sentences) > 1 { // join multi-sentence messages
				text := make([]byte, 0, 2*len(text))
				for _, s := range m.Sentences {
					text = append(text, s.Text...)
				}
			}

			for token, c := range connections {
				select {
				case c.send <- text: // send message unless channel is full
					c.fullFor = 0 // reset on success
				default: // register dropped message
					c.fullFor++
					if c.fullFor == CLOSE_FORWARDER_AFTER { // cancel forwarder
						close(c.send)
						delete(connections, token)
					}
				}
			}
		case token := <-closer: // forwarder stopped on it's own
			delete(connections, token)
		case to := <-add: // create new forwarder
			c := make(chan []byte, FORWARDER_CHANNEL_CAP)
			prevToken++
			connections[prevToken] = forwardConn{c, 0}
			go forwardTo(to, c, prevToken, closer)
		}
	}
}

// Wrapper around forwarders created by ForwarderManager().
// Returns when there is an error or manger cancels it.
func forwardTo(to NewForwarder, messages <-chan []byte,
	token int64, closer chan<- int64) {
get:
	for write := range messages {
		written := 0
		for written < len(write) {
			n, err := to.Write(write[written:])
			written += n
			if err != nil && err != io.ErrShortWrite {
				if !strings.Contains(err.Error(), "broken pipe") {
					Log.Debug("forwarder %d Write() error: %s", token, err.Error())
				}
				closer <- token
				break get
			}
		}
	}
	// Don't send token if channel was closed: manager has already removed us.
	err := to.Close()
	if err != nil {
		Log.Debug("forwarder %d Close() error: %s", token, err.Error())
	}
}
