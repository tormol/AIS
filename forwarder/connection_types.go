package forwarder

import (
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	l "github.com/tormol/AIS/logger"
)

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

// ToHTTP sets up the writer for forwarding and passes it to add.
// Doesn't return until the client disconnects or there is an I/O error.
// Packets sent through this will be concatenated and split as the ResponseWriter sees fit.
func ToHTTP(sendTo chan<- Conn, w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Transfer-Encoding", "chunked")
	// Need to stay in this function while the connection lasts,
	// so there is no point in trying to extract (Hijack) a TCPConn.
	w.WriteHeader(http.StatusOK)
	hfc := &httpForwarderConn{w, make(chan struct{})}
	sendTo <- hfc
	// TODO detect add closed
	<-hfc.ended
}

// TCPServer listens for TCP connections and passes the connection to add.
// Never returns, but any IO error from ResolveTCPAddr(), ListenTCP()
// or AcceptTCP() is fatal.
// As TCP is stream-oriented, packets might be split or merged
// even without delays to send bigger and fewer packets.
func TCPServer(log *l.Logger, serveAddr string, add chan<- Conn) {
	a, err := net.ResolveTCPAddr("tcp", serveAddr)
	log.FatalIfErr(err, "resolve forwarding TCP address")
	l, err := net.ListenTCP("tcp", a)
	log.FatalIfErr(err, "listen for TCP")
	defer func() {
		err = l.Close()
		if err != nil {
			log.Error("Error closing TCP server: %s", err.Error())
		}
	}()
	for {
		conn, err := l.AcceptTCP()
		log.FatalIfErr(err, "accept forwarding TCP connection")
		add <- conn // TCPConn implements WriteCloser
	}
}

const (
	udpRunning = 0
	udpStop    = iota
	udpStopped = iota
)

// Because UDP is fire-and-forget, the client stopping listening won't cause an
// error, so we could easily end up sending forever.
// Therefore we need to time out after a while.
type udpForwarderConn struct {
	listener *net.UDPConn // immutable, used by forwarder
	to       *net.UDPAddr // immutable, used by forwarder
	flag     int32        // see consts
	timeout  time.Time    // not atomic; controlled by server
}

func (ufc *udpForwarderConn) Write(slice []byte) (int, error) {
	if atomic.CompareAndSwapInt32(&ufc.flag, udpStop, udpStopped) {
		return 0, io.EOF
	}
	n, err := ufc.listener.WriteToUDP(slice, ufc.to)
	if err != nil {
		atomic.StoreInt32(&ufc.flag, udpStopped)
	}
	return n, err
}
func (ufc *udpForwarderConn) Close() error {
	atomic.StoreInt32(&ufc.flag, udpStopped)
	return nil
}

// UDPServer listens for UDP packets and starts / stops / times out forwarders
// Never returns, but any IO error from ResolveUDPAddr(), ListenUDP()
// or ReadFromUDP() is fatal.
// Packets will never be merged or split, but
// if the receivers buffer is too small it might not see everything.
func UDPServer(log *l.Logger, listenAddr string, add chan<- Conn) {
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	log.FatalIfErr(err, "resolve forwarding UDP address")
	listener, err := net.ListenUDP("udp", laddr)
	log.FatalIfErr(err, "listen for UDP")

	connections := make(map[string]*udpForwarderConn)
	stop := time.NewTicker(1 * time.Second).C
	start := make(chan *net.UDPAddr, 16)

	// Receive UDP packets and send the source addr to a channel that can be selected over
	go func() {
		defer func() {
			log.FatalIfErr(listener.Close(), "close forwarder UDP server")
		}()
		buf := make([]byte, 32) // avoid an empty buffer in case it could cause issues
		for {
			_, from, err := listener.ReadFromUDP(buf)
			log.FatalIfErr(err, "accept forwarding UDP connection")
			start <- from
		}
	}()

	for {
		select {
		case from := <-start:
			now := time.Now()
			timeout := now.Add(UDPTimeout)
			fromAddrStr := from.String()
			ufc := connections[fromAddrStr]
			if ufc == nil { // create new connection
				ufc = &udpForwarderConn{
					listener: listener,
					to:       from,
					flag:     udpRunning,
					timeout:  timeout,
				}
				connections[fromAddrStr] = ufc
				add <- ufc
			} else if atomic.LoadInt32(&ufc.flag) == udpRunning {
				// reset timeout if it hasn't been stopped
				ufc.timeout = timeout
			} else { // reset and restart if there somehow was an error
				ufc.flag = udpRunning
				ufc.timeout = timeout
				add <- ufc
			}
		case now := <-stop:
			// stop forwarding to clients we haven't heard anything from
			for k, ufc := range connections {
				if now.After(ufc.timeout) {
					// only tell the forwarder to stop if it's running
					atomic.CompareAndSwapInt32(&ufc.flag, udpRunning, udpStop)
					delete(connections, k)
				}
			}
		}
	}
}
