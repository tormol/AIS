package nmeais

import (
	"sync"
	"time"
)

// DuplicateTester is a tool for filtering out messages received from multiple AIS ssources.
// It does this by comparing new message against all recently checked messages.
// This means identical messages from the same source will also be filtered.
// What's considered recent is controlled by a parmater to the constructor,
// and a package might be comparaed against all received within the double of that.
// It uses internal locking, which makes it safe to share instances between goroutines.
type DuplicateTester struct {
	active  map[string]struct{} //Points to the oldest map (the one where incoming messages are being tested against)
	pending map[string]struct{} //Points to the pending map
	mu      sync.Mutex          //Not a pointer because copying the struct will break tableOrganizer anyway.
	stop    bool                //tells tableOrganizer to stop
}

/*
NewDuplicateTester creates a new DuplicateTester and starts a goroutine that
periodically removes old messages.

input:
	minKeepAlive - How long the messages should at least be kept in the map
				   E.g. 5 seconds -> a new message is tested for duplicates
				   among all the messages recieved within the last 5 to 10 seconds
*/
func NewDuplicateTester(minKeepAlive time.Duration) *DuplicateTester {
	dt := &DuplicateTester{
		active:  make(map[string]struct{}, 0),
		pending: make(map[string]struct{}, 0),
		mu:      sync.Mutex{},
	}
	go tableOrganizer(dt, minKeepAlive)
	return dt
}

/*
TODO:
	- could be improved by occasionally testing the amount of messages recieved per seconds in order to allocate a more suitable amount of memory for the maps
*/
//this function organizes the creation and resetting of the maps. It is run in its own goroutine
func tableOrganizer(dt *DuplicateTester, keepAlive time.Duration) {
	for {
		time.Sleep(keepAlive) // every keepAlive, one table is cleared, and the other Table is set as active
		dt.mu.Lock()
		empty := make(map[string]struct{}, len(dt.active)+100) // +100 to account for uneven traffic
		dt.active = dt.pending                                 // set new active
		dt.pending = empty                                     // the "pending"-map is now a empty map
		stop := dt.stop
		dt.mu.Unlock()
		if stop { // prevent deadlock, even if that would make bugs more noticable.
			return
		}
	}
}

// Close tells the internal goroutine to stop.
func (dt *DuplicateTester) Close() {
	dt.mu.Lock()
	dt.stop = true
	dt.mu.Unlock()
}

/*
IsDuplicate compares msg against all messages passed to IsDuplicate within
the last 1x to 20 minKeepAlive.

Input: 	msg    - Only the raw text of the first sentence is used. (for speed and simplicity)
Output:	exists - true if the message is previously known
               - false if the message is new
*/
func (dt *DuplicateTester) IsDuplicate(msg *Message) bool {
	dt.mu.Lock()
	s := msg.Sentences()[0].Text
	_, exists := dt.active[s]
	if !exists { //The message is not previously known
		dt.active[s] = struct{}{}  // mark the message as known
		dt.pending[s] = struct{}{} // to both maps
	}
	dt.mu.Unlock()
	return exists
}
