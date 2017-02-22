// duplicateTester
//Note: This currently only works for strings...
/*
Exported methods & functions:
	func NewDuplicateTester(keepAlive int) *DuplicateTester
	func (dt *DuplicateTester) IsRepeated(message string) bool
*/

package main

import (
	"sync"
	"time"
)

//Keeps track of which map is active
type DuplicateTester struct {
	active  map[string]struct{} //Points to the oldest map (the one where incoming messages are being tested against)
	pending map[string]struct{} //Points to the pending map
	mu      sync.Mutex          //Not a pointer because copying the struct will break tableOrganizer anyway.
}

/*
input:
	minKeepAlive - How long the messages should at leastbe kept in the map
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
		time.Sleep(keepAlive) // every 'keepAlive' seconds; one of the tables are reset, and the other Table is set as active
		dt.mu.Lock()
		empty := make(map[string]struct{}, len(dt.active)+100) // to account for uneven traffic
		dt.active = dt.pending                                 // set new active
		dt.pending = empty                                     // the "pending"-map is now a empty map
		dt.mu.Unlock()
	}
}

/*
Input: 	msg    - Only the raw text of the first sentence is used. (for speed and simplicity)
Output:	exists - true if the message is previously known
               - false if the message is new
*/
func (dt *DuplicateTester) IsRepeated(msg *Message) bool {
	dt.mu.Lock()
	s := string(msg.Sentences[0].Text)
	_, exists := dt.active[s]
	if !exists { //The message is not previously known
		dt.active[s] = struct{}{} // mark the message as known
		dt.pending[s] = struct{}{}
	}
	dt.mu.Unlock()
	return exists
}
