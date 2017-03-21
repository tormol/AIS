package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tormol/AIS/logger"
	"github.com/tormol/AIS/nmeais"
)

const (
	// MergeHistory is the minimum time messages are kept to be compared againts new messages.
	MergeHistory = 2 * time.Second
)

// SourceMerger is a wrapper around nmeais.DuplicateTester that does logging and forwarding.
// It is synchronized internally so messages can be sumbitted from multiple goroutines.
type SourceMerger struct {
	// if DuplicateTester was inlined we could have used its mutex instead of atomic operations,
	// but the separation of concerns is worth it.
	logger            *logger.Logger
	toForwarder       chan<- []byte
	toArchive         chan<- *nmeais.Message
	dt                *nmeais.DuplicateTester
	periodForwarded   [28]uint64 // use atomic operations
	periodDuplicates  [28]uint64 // use atomic operations
	allTimeForwarded  [28]uint64 // only accessed by logger
	allTimeDuplicates [28]uint64 // only accessed by logger
	// These four arrays together take nearly a kilobyte
}

// NewSourceMerger returns a reference because it starts an internal goroutine.
func NewSourceMerger(log *logger.Logger,
	toForwarder chan<- []byte, toArchive chan<- *nmeais.Message,
) *SourceMerger {
	sm := &SourceMerger{
		logger:      log,
		dt:          nmeais.NewDuplicateTester(MergeHistory),
		toForwarder: toForwarder,
		toArchive:   toArchive,
		// remaining are zero
	}
	log.AddPeriodicLogger("SourceMerger", 5*time.Minute, func(l *logger.Logger, d time.Duration) {
		pTotal, aTotal := uint64(0), uint64(0)
		indexes, pf, pd := "Type:      ", "Forwarded: ", "Duplicates:"
		af, ad := pf, pd
		for i := 0; i < 28; i++ {
			pfn := atomic.SwapUint64(&sm.periodForwarded[i], 0) // load and reset
			pdn := atomic.SwapUint64(&sm.periodDuplicates[i], 0)
			afn := sm.allTimeForwarded[i]
			adn := sm.allTimeDuplicates[i]
			sm.allTimeForwarded[i] += pfn
			sm.allTimeDuplicates[i] += pdn
			pTotal += pfn + pdn
			aTotal += afn + adn
			if pfn > 0 { // the first one cannot be a duplicate
				indexes += fmt.Sprintf(" %5d", i)
				pf += fmt.Sprintf(" %5d", pfn)
				pd += fmt.Sprintf(" %5d", pdn)
				af += fmt.Sprintf(" %5d", afn)
				ad += fmt.Sprintf(" %5d", adn)
			}
		}
		log.Debug("SourceMerger: total %d (all time: %d), per type:\n%s\n%s\n%s\n%s\n%s",
			pTotal, aTotal, indexes, pf, pd, af, ad,
		)
	})
	return sm
}

// Accept logs m's type and sends it to forwarder and Archive if it haen't a duplicate.
func (sm *SourceMerger) Accept(m *nmeais.Message) {
	t := m.Type()
	if t > 27 {
		t = 0 // unknown
	}
	if sm.dt.IsDuplicate(m) {
		atomic.AddUint64(&sm.periodDuplicates[t], 1)
	} else {
		atomic.AddUint64(&sm.periodForwarded[t], 1)
		sm.toForwarder <- []byte(m.Text())
		sm.toArchive <- m // TODO move parts of archive.Saver here
	}
}

// Close closes the channel which makes future calls to Accept block forever.
func (sm *SourceMerger) Close() {
	sm.dt.Close()
	close(sm.toForwarder)
	close(sm.toArchive)
	sm.logger.RemovePeriodicLogger("source_merger")
}
