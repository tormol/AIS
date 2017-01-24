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
	t  *Table      //Points to the oldest table (the table where incoming messages are being tested against)
	mu *sync.Mutex //mutex lock
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
										E.g. 5 seconds -> a new message is tested for duplicates among all the messages recieved within the last 5 seconds(or more) (first Table stores messages for at least 2x keepAlive)
*/
func NewDuplicateTester(keepAlive int) *DuplicateTester {
	a := Table{make(map[string]bool, 0), &sync.Mutex{}} //creating the first Table

	dt := DuplicateTester{&a, &sync.Mutex{}} // table "a" is set as the active Table
	go tableOrganizer(&dt, (time.Duration(keepAlive) * time.Second), &a)
	return &dt
}

/*
TODO:
	- could be improved by occasionally testing the amount of messages recieved per seconds in order to allocate a more suitable amount of memory for the maps
*/
//this function organizes the creation and resetting of the maps. It is run in its own goroutine
func tableOrganizer(dt *DuplicateTester, keepAlive time.Duration, a *Table) {
	//The first Table is already made. The tables are called 'a' and 'b'

	time.Sleep(keepAlive) //waiting a specified number of seconds before making the second Table
	b := Table{make(map[string]bool, 0), &sync.Mutex{}}
	for {
		time.Sleep(keepAlive) // every 'keepAlive' seconds; one of the tables are reset, and the other Table is set as active
		(*dt).mu.Lock()
		if (*dt).t == a {
			(*dt).t = &b
			a.reset() //go a.reset() ?
			//fmt.Println("reset a") //for debugging
		} else {
			(*dt).t = a
			b.reset()
			//fmt.Println("reset b") //for debugging
		}
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
	(*dt).t.mu.Lock()
	r := true
	if _, ok := (*dt).t.m[message]; !ok { //The message is not previously known
		(*dt).t.m[message] = true // mark the message as known
		r = false
	}
	(*dt).t.mu.Unlock()
	(*dt).mu.Unlock()
	return r
}
