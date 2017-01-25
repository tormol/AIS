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

type Table struct {
	m  map[string]bool //map
	mu *sync.Mutex     //mutex lock
}

//Keeps track of which Table is active
type DuplicateTester struct {
	active  *Table      //Points to the oldest table (the table where incoming messages are being tested against)
	pending *Table      //Points to the pending table
	mu      *sync.Mutex //mutex lock
}

//"Resets" a Table
func (t *Table) reset() {
	(*t).mu.Lock()
	(*t).m = make(map[string]bool, 0) // set the Table to point to a new (empty) map
	(*t).mu.Unlock()
}

/*
input:
	keepAlive	-	time.Duration	-	how long the messages should be kept in the map
										E.g. 5 seconds -> a new message is tested for duplicates among all the messages recieved within the last 5 seconds(or more)
*/
func NewDuplicateTester(keepAlive int) *DuplicateTester {
	a := Table{make(map[string]bool, 0), &sync.Mutex{}} //creating the first Table
	b := Table{make(map[string]bool, 0), &sync.Mutex{}} //creating the second Table

	dt := DuplicateTester{&a, &b, &sync.Mutex{}} // table "a" is set as the active Table
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
		tmp := (*dt).active
		(*dt).active = (*dt).pending // set new active
		tmp.reset()
		(*dt).pending = tmp
		//fmt.Println("Switched Table") // for debugging
		(*dt).mu.Unlock()
	}
}

//TODO this looks ugly...
/*
Input: 	message	-	string	-	the raw AIS message as a string (or any other string...)
Output:	r	-	boolean	-	true if the message is previously known
						-	false if the message is new
*/
func (dt *DuplicateTester) IsRepeated(message string) bool {
	(*dt).mu.Lock()
	(*dt).active.mu.Lock()
	r := true
	if _, ok := (*dt).active.m[message]; !ok { //The message is not previously known
		(*dt).active.m[message] = true // mark the message as known
		(*dt).pending.mu.Lock()
		(*dt).pending.m[message] = true
		(*dt).pending.mu.Unlock()
		r = false
	}
	(*dt).active.mu.Unlock()
	(*dt).mu.Unlock()
	return r
}
