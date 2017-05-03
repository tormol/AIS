package forwarder

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"testing"
	"time"

	l "github.com/tormol/AIS/logger"
)

// A forwarder.Conn mock
type managerTester struct {
	t             *testing.T
	closer        chan<- struct{}
	packetIndex   int
	inPacketIndex int
	closed        bool
	packets       [][]byte
	id            int
	maxDelay      time.Duration
	errAfter      bool
}

func (mt *managerTester) Write(packet []byte) (int, error) {
	if mt.packetIndex == len(mt.packets) {
		if mt.errAfter {
			return len(packet) / 2, errors.New("I/O error")
		}
		mt.t.Errorf("conn %d: Connection received too many packets", mt.id)
		return 0, errors.New("Too many packets")
	}

	// varying but non-random and integratable delay
	// the if makes it much easier to integrate
	if mt.inPacketIndex == 0 {
		delay := float64(mt.maxDelay/2) * (1 + math.Sin(float64(mt.packetIndex)/10))
		time.Sleep(time.Duration(delay))
	}

	want := mt.packets[mt.packetIndex][mt.inPacketIndex:]
	if !bytes.Equal(want, packet) {
		mt.closer <- struct{}{}
		mt.t.Errorf("conn %d packet %d:\nwanted %s\n   got %s",
			mt.id, mt.packetIndex, want, packet)
		return len(packet), errors.New("Wrong Packet")
	}
	if len(want) > 90 || len(want) == 2 {
		mt.inPacketIndex += len(want) / 2
		return len(want) / 2, io.ErrShortWrite
	}
	mt.packetIndex++
	mt.inPacketIndex = 0
	if mt.packetIndex == len(mt.packets) {
		mt.closer <- struct{}{}
	}
	return len(packet), nil
}

func (mt *managerTester) Close() error {
	if mt.packetIndex != len(mt.packets) {
		mt.t.Errorf("conn %d: Wanted %d packets, got %d",
			mt.id, len(mt.packets), mt.packetIndex)
	}
	mt.closed = true
	return nil
}

// Tests manager with multiple Conns with unequal and varying delays.
// Tests write error handling and manager closing, but not
// Close() error handling or dropping packets before dropping the connection.
func TestManager(t *testing.T) {
	packets := make([][]byte, 100)
	for pi := range packets {
		packets[pi] = make([]byte, pi)
		for bi := range packets[pi] {
			packets[pi][bi] = "1234567890"[bi%10]
		}
	}

	closer := make(chan struct{}, 10)
	running := 0
	nt := func(want, maxDelay int, errAfter bool) *managerTester {
		running++
		return &managerTester{t, closer, 0, 0, false,
			packets[:want], running, time.Duration(maxDelay) * time.Millisecond, errAfter,
		}
	}
	conns := [...]*managerTester{
		nt(len(packets), 20, false),
		nt(len(packets), 50, false),
		nt(len(packets), 60, false),
		nt(6, 99, true),
	}

	add := make(chan Conn)
	sender := make(chan []byte, 10)
	l := l.NewLogger(os.Stderr, l.Info)
	go Manager(l, sender, add)
	for _, c := range conns {
		add <- c
	}

	// the sum of time up to p packets is int (maxmaxdelay/2)(sin(p/10)+1) dp
	// = (p-10 cos(p/10))*maxmaxdelay/2
	// test that channels are flushed before closing the connection is closed:
	waitFor := float64(len(packets) - ConnChannelCap/2)
	// the sum of time for that is int_0^waitFor (maxmaxdelay/2)(sin(p/10)+1) dp
	// = (waitfor-10 cos(waitfor/10))*maxmaxdelay/2 - (0-10 cos(0/10))*maxmaxdelay/2
	// = (waitfor-10 cos(waitfor/10) + 10)*maxmaxdelay/2
	duration := (waitFor - 10*math.Cos(waitFor/10) + 10) * float64(conns[2].maxDelay) / 2
	avg := time.Duration(duration) / time.Duration(len(packets))
	for _, p := range packets {
		time.Sleep(avg)
		sender <- p
	}
	for running > 0 {
		<-closer
		running--
	}

	close(sender)
	time.Sleep(500 * time.Millisecond) // wait until it's finished
	for i, mt := range conns {
		if mt.packetIndex != len(mt.packets) {
			t.Errorf("Conn %d had only received %d out of %d packets",
				i, mt.packetIndex, len(mt.packets))
		}
		if !mt.closed {
			t.Errorf("Conn %d not closed", mt.id)
		}
	}
}
