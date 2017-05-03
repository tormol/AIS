package forwarder

import (
	"io"
	"strings"
	"time"

	l "github.com/tormol/AIS/logger"
)

const (
	// ConnChannelCap is the capacity of the channel to each connection wrapper
	ConnChannelCap = 20
	// CloseConnAfter is the maximum number of packets that can be skipped in a
	// row due to the channel being full before the connection is dropped
	CloseConnAfter = 20
	// UDPTimeout is how long packets will be sent for after a received packet
	UDPTimeout = 5 * time.Second
)

// ClientLogLevel controls weither client IO errors should be logged
var ClientLogLevel = l.Ignore

// Conn abstracts away the actual trait from other files
type Conn interface {
	io.WriteCloser
}

// monotonically increasing ID sent when a forwarder stops on its own.
type token uint64

// Info about forwarders stored by ForwarderManager()
type forwardConn struct {
	send    chan<- []byte
	fullFor int64
}

// Manager starts new forwarders and cancels them if they stop consuming packets.
// Returns when packets is closed.
// forwarders do not merge buffered packets, but TCP-based connections might
// both merge and split packets.
func Manager(log *l.Logger, packets <-chan []byte, add <-chan Conn) {
	prevToken := token(0)
	connections := make(map[token]forwardConn)
	closer := make(chan token) // unbuffered
	for {
		select {
		case p, notClosed := <-packets: // new message to forward
			if !notClosed {
				// close all connections and stop
				for _, c := range connections {
					close(c.send)
				}
				return
			}
			// forward packet to all channels
			for token, c := range connections {
				select {
				case c.send <- p: // send message unless channel is full
					c.fullFor = 0 // reset on success
				default: // register dropped message
					c.fullFor++
					if c.fullFor == CloseConnAfter { // cancel forwarder
						close(c.send)
						delete(connections, token)
					}
				}
			}
		case t := <-closer: // a forwarder stopped on its own
			delete(connections, t)
		case to := <-add: // create new forwarder
			c := make(chan []byte, ConnChannelCap)
			prevToken++
			connections[prevToken] = forwardConn{c, 0}
			go forwardTo(log, to, c, prevToken, closer)
		}
	}
}

// Wrapper around forwarders created by Manager().
// Returns when there is an error or manager cancels it.
func forwardTo(log *l.Logger, to Conn, packets <-chan []byte,
	token token, closer chan<- token) {
get:
	for packet := range packets {
		for {
			sent, err := to.Write(packet)
			if err != nil && err != io.ErrShortWrite {
				if !strings.Contains(err.Error(), "broken pipe") {
					log.Log(ClientLogLevel, "forwarder %d Write() error: %s", token, err.Error())
				}
				closer <- token
				break get
			} else if sent == len(packet) {
				break // complete
			} else {
				packet = packet[sent:]
			}
		}
	}
	// Don't send token if channel was closed: manager has already removed us.
	err := to.Close()
	if err != nil {
		log.Log(ClientLogLevel, "forwarder %d Close() error: %s", token, err.Error())
	}
}
