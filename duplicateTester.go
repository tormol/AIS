// duplicateTester
//Note: This currently only works for strings...
/*
Exported methods & functions:
	func NewDuplicateTester(keepAlive int) *DuplicateTester
	func (dt *DuplicateTester) IsRepeated(message string) bool
*/

package AIS

import (
	//"fmt" //only for debugging
	"sync"
	"time"
)

//Keeps track of which map is active
type DuplicateTester struct {
	active  map[string]bool //Points to the oldest map (the one where incoming messages are being tested against)
	pending map[string]bool //Points to the pending map
	mu      *sync.Mutex     //mutex lock
}

/*
input:
	keepAlive	-	time.Duration	-	how long the messages should be kept in the map
										E.g. 5 seconds -> a new message is tested for duplicates among all the messages recieved within the last 5 seconds(or more)
*/
func NewDuplicateTester(keepAlive int) *DuplicateTester {
	dt := DuplicateTester{make(map[string]bool, 0), make(map[string]bool, 0), &sync.Mutex{}} // Creates two maps
	go tableOrganizer(&dt, (time.Duration(keepAlive) * time.Second))
	return &dt
}

/*
TODO:
	- could be improved by occasionally testing the amount of messages recieved per seconds in order to allocate a more suitable amount of memory for the maps
*/
//this function organizes the creation and resetting of the maps. It is run in its own goroutine
func tableOrganizer(dt *DuplicateTester, keepAlive time.Duration) {
	for {
		time.Sleep(keepAlive) // every 'keepAlive' seconds; one of the tables are reset, and the other Table is set as active
		(*dt).mu.Lock()
		(*dt).active = (*dt).pending             // set new active
		(*dt).pending = make(map[string]bool, 0) // the "pending"-map is now a empty map
		//fmt.Println("Switched Table")            //for debugging
		(*dt).mu.Unlock()
	}
}

/*
Input: 	message	-	string	-	the raw AIS message as a string (or any other string...)
Output:	r	-	boolean	-	true if the message is previously known
						-	false if the message is new
*/
func (dt *DuplicateTester) IsRepeated(message string) bool {
	(*dt).mu.Lock()
	r := true
	if _, ok := (*dt).active[message]; !ok { //The message is not previously known
		(*dt).active[message] = true // mark the message as known
		(*dt).pending[message] = true
		r = false
	}
	(*dt).mu.Unlock()
	return r
}
