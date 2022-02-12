package storage

/*
An implementation of a 2-dimentional R*-Tree used for storing <lat,long> coordinates of boats. See references [0] and [9] for description of the datastructure (haven't followed the instructions 100%)

Notes:
    - MBR - Minimum Bounding Rectangle
    - The FindWithin() & FindAll() function returns the coordinates (& the mmsi number?) for the boat. More info about the boats are found when clicking the leafletjs markers/ querying the API
    - The height of a node will never change, but its level will increase as the root is split
    - All leaf nodes must be on the same level
    - Internal nodes contains entries of the form <childNode, mbr>
    - Leaf nodes contains entries of the form  <mbr, mmsi>
     - Wiki: best performance has been experienced with a minimum fill of 30%–40% of the maximum number of entries
    - Boats are stored as zero-area rectangles instead of points, because it works better with the R*tree
*/

import (
	"errors"
	"log"
	"sort"

	"github.com/tormol/AIS/geo"
)

const RTree_M = 5 //max entries per node.
const RTree_m = 2 //min entries per node.	40% of M is best

// RTree is a two-dimensional R*-tree implementation with float64 positions and uint32 values
type RTree struct {
	root       *node
	numOfBoats int
}

// NumOfBoats return the total number of boats stored in the structure.
func (rt *RTree) NumOfBoats() int {
	return rt.numOfBoats
}

// Match is used to store a match found when searching the tree.
type Match struct {
	MMSI uint32
	Lat  float64
	Long float64
}

type node struct {
	parent  *node   //Points to parent node
	entries []entry //Array of all the node's entries    (should have a default length of M+1)
	height  int     //Height of the node ( = number of edges between node and a leafnode)
}

// isLeaf returns true of the node is a leafnode.
func (n *node) isLeaf() bool { return n.height == 0 }

// Needed for node to be sortable [11]:
type byLat []entry  // for sorting by Latitude
type byLong []entry // for sorting by Longitude

func (e byLat) Len() int      { return len(e) }
func (e byLat) Swap(i, j int) { e[i], e[j] = e[j], e[i] }
func (e byLat) Less(i, j int) bool { //first sorted by min, then if tie, by max
	if e[i].mbr.Min().Lat < e[j].mbr.Min().Lat {
		return true
	} else if e[i].mbr.Min().Lat == e[j].mbr.Min().Lat {
		return e[i].mbr.Max().Lat < e[j].mbr.Max().Lat
	}
	return false
}

func (e byLong) Len() int      { return len(e) }
func (e byLong) Swap(i, j int) { e[i], e[j] = e[j], e[i] }
func (e byLong) Less(i, j int) bool { //first sorted by min, then if tie by max
	if e[i].mbr.Min().Long < e[j].mbr.Min().Long {
		return true
	} else if e[i].mbr.Min().Long == e[j].mbr.Min().Long {
		return e[i].mbr.Max().Long < e[j].mbr.Max().Long
	}
	return false
}

/* As described in [9]:
- A non-leaf node contains entries of the form (child_pointer, rectangle)
- A leaf node contains entries of the form (Object_ID, rectangle)
*/
type entry struct {
	mbr   *geo.Rectangle //Points to the MBR containing all the children of this entry
	child *node          //Points to the node (only used in internal nodes)
	mmsi  uint32         //The mmsi number of the boat (only used in leafnode-entries)
	dist  float64        //The distance from center of mbr to center of parents mbr    (used for the reInsert algorithm)
}

/*
   Needed for sorting a list of entries by the distance from their center
   to the center of the "parent node" mbr.    (used by reInsert algorithm)
*/
type byDist []entry

func (e byDist) Len() int           { return len(e) }
func (e byDist) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e byDist) Less(i, j int) bool { return e[i].dist < e[j].dist }

// NewRTree returns a pointer to a new R-Tree object.
func NewRTree() *RTree { //TODO could take M (and m) as input?
	return &RTree{
		root: &node{
			parent:  nil,
			entries: make([]entry, 0, RTree_M+1),
			height:  0,
		},
	}
}

// InsertData inserts a new boat into the tree structure.
func (rt *RTree) InsertData(lat, long float64, mmsi uint32) error {
	r, err := geo.NewRectangle(lat, long, lat, long)
	if err != nil {
		return err
	}
	newEntry := entry{ //Dont have to set all the parameters... the rest will be set to its null-value
		mbr:  r,
		mmsi: mmsi,
	}
	//[ID1] Insert starting with the leaf height as parameter
	rt.insert(0, newEntry, true)
	rt.numOfBoats++
	return nil
}

// insert inserts an entry into a node at a given height.
func (rt *RTree) insert(height int, newEntry entry, first bool) { //first is needed in case of overflowTreatment, it should normaly be true
	//[I1]    ChooseSubtree with height as a parameter to find the node N
	n := rt.chooseSubtree(newEntry.mbr, height)
	//If an internal entry is re-inserted, the node's parent pointer must be updated
	if height >= 1 {
		newEntry.child.parent = n
	}
	//[I2]    Append newEntry to n if room, else call OverflowTreatment [for reinsertion or split]
	n.entries = append(n.entries, newEntry)
	if len(n.entries) >= RTree_M+1 { // n is full -> call overflowTreatment
		didSplit, nn := rt.overflowTreatment(n, first) //OT finds the appropriate height from n.height
		if didSplit {
			//[I3]    if OverflowTreatment was called and a split was performed: propagate OT upwards
			if nn.height == rt.root.height { // if root was split: create a new root
				newRoot := node{
					parent:  nil,
					entries: make([]entry, 0, RTree_M+1),
					height:  rt.root.height + 1,
				}
				nEntry := entry{mbr: n.recalculateMBR(), child: n}
				nnEntry := entry{mbr: nn.recalculateMBR(), child: nn}
				newRoot.entries = append(newRoot.entries, nEntry)
				newRoot.entries = append(newRoot.entries, nnEntry)
				n.parent = &newRoot
				nn.parent = &newRoot
				rt.root = &newRoot
				//fmt.Printf("Root was split...^ new height is %d\n", newRoot.height)
				return //The root has no MBR, so there is no need to adjust any MBRs
			}
			// n was split into n & nn -> insert nn into the tree at the same height
			rt.insert(nn.height+1, entry{mbr: nn.recalculateMBR(), child: nn}, true)
		}
	}
	//[I4]    Adjust all MBR in the insertion path
	for n.height < rt.root.height {
		pIdx, err := n.parentEntriesIdx()
		CheckErr(err, "insert had some trouble adjusting the MBR...")
		n.parent.entries[pIdx].mbr = n.recalculateMBR()
		n = n.parent
	}
	return
}

// overflowTreatment handles the overflowing node n.
// It will first try a reinsert, then do a split.
func (rt *RTree) overflowTreatment(n *node, first bool) (bool, *node) { //returns if n wasSplit, and nn    (false -> reInserted )
	//[OT1]    if height is not root && this is first call of OT in given height during insertion: reInsert. else: split
	if first && n.height < rt.root.height {
		rt.reInsert(n)
		return false, nil
	} else { // The entry has been inserted before -> split the node
		nn, err := n.split()
		CheckErr(err, "overflowTreatment failed to split a node")
		return true, nn
	}
}

// reInsert is uses to re-insert some of the entries of the node.
// It is used when the node is full.
func (rt *RTree) reInsert(n *node) {
	//[RI1] for all M+1 entries: compute distance between their center and the center of the mbr of n
	//    Finding the center of the MBR of n
	i, err := n.parentEntriesIdx()
	CheckErr(err, "reInsert had some trouble locating the entry in the parent node")
	centerOfMBR := n.parent.entries[i].mbr.Center()
	//    Computing the distance for all entries in n
	for _, ent := range n.entries {
		ent.dist = ent.mbr.Center().DistanceTo(centerOfMBR)
	}
	//[RI2] sort the entries by distance in decreasing order
	sort.Sort(sort.Reverse(byDist(n.entries)))
	//[RI3]    remove the first p entries from n, and adjust mbr of n
	f := (RTree_M * 0.3) //30% of M performs best according to [9]
	p := int(f)
	tmp := make([]entry, p)
	copy(tmp, n.entries[:p])
	n.entries = n.entries[p:] //TODO now the cap of n.entries is only 8...
	newMBR := n.recalculateMBR()
	n.parent.entries[i].mbr = newMBR
	//[RI4]    starting with min distance: invoke insert to reinsert the entries
	for k := len(tmp) - 1; k >= 0; k-- {
		rt.insert(n.height, tmp[k], false) // "first" is set to false because the entry has previously been inserted
	}
}

// chooseSubtree chooses the leaf node (or the best node of a given height) in which to place a new entry.
func (rt *RTree) chooseSubtree(r *geo.Rectangle, height int) *node {
	n := rt.root                           //CS1
	for !n.isLeaf() && n.height > height { //CS2        n.height gets lower for every iteration
		bestChild := n.entries[0]
		pointsToLeaves := false
		if n.height == 1 {
			pointsToLeaves = true
		}
		var bestDifference float64 //must be reset for each node n
		if pointsToLeaves {
			bestDifference = bestChild.overlapChangeWith(r)
		} else {
			bestDifference = bestChild.mbr.AreaDifference(bestChild.mbr.MBRWith(r))
		}
		for i := 1; i < len(n.entries); i++ {
			e := n.entries[i]
			if pointsToLeaves { //childpointer points to leaves -> [Determine the minimum overlap cost]
				overlapDifference := e.overlapChangeWith(r)
				if overlapDifference <= bestDifference {
					if overlapDifference < bestDifference { //strictly smaller
						bestDifference = overlapDifference
						bestChild = e //CS3 set new bestChild, repeat from CS2
					} else { //tie -> choose the entry whose rectangle needs least area enlargement
						eNew := e.mbr.MBRWith(r).AreaDifference(e.mbr)
						eOld := bestChild.mbr.MBRWith(r).AreaDifference(bestChild.mbr)
						if eNew < eOld {
							bestDifference = overlapDifference
							bestChild = e //CS3 set new bestChild, repeat from CS2
						} else if e.mbr.Area() < bestChild.mbr.Area() { //if tie again: -> choose the entry with the smallest MBR
							bestDifference = overlapDifference
							bestChild = e //CS3 set new bestChild, repeat from CS2
						} //else the bestChild is kept
					}
				}
			} else { //childpointer do not point to leaves -> choose the child-node whose rectangle needs least enlargement to include r
				newMBR := e.mbr.MBRWith(r)
				areaDifference := e.mbr.AreaDifference(newMBR)
				if areaDifference <= bestDifference { //we have a new best (or a tie)
					if areaDifference < bestDifference {
						bestDifference = areaDifference //CS3 set new bestChild, repeat from CS2
						bestChild = e
					} else if e.mbr.Area() < bestChild.mbr.Area() { // change in MBR is a tie -> keep the rectangle with the smallest area
						bestDifference = areaDifference //CS3 set new bestChild, repeat from CS2
						bestChild = e
					}
				}
			}
		}
		n = bestChild.child
	}
	return n
}

// overlapChangeWith calculates how much overlap enlargement it takes to include the given rectangle.
func (e *entry) overlapChangeWith(r *geo.Rectangle) float64 {
	return e.mbr.OverlapWith(r)
}

// split() will split a node in order to add a new entry to a full node (using the R*Tree algorithm)[9].
func (n *node) split() (*node, error) {
	// the goal is to partition the set of M+1 entries into two groups
	// sorts the entries by the best axis, and finds the best index to split into two distributions
	if len(n.entries) != RTree_M+1 {
		return nil, errors.New("Cannot split: node n does not contain M+1 entries")
	}
	k := n.chooseSplitAxis()
	group1 := make([]entry, 0, RTree_M+1)
	group2 := make([]entry, 0, RTree_M+1)
	nn := &node{
		parent:  n.parent,
		entries: []entry{},
		height:  n.height,
	}
	for i, e := range n.entries {
		if i < RTree_m-1+k {
			group1 = append(group1, e)
		} else {
			group2 = append(group2, e)
			if e.child != nil { //update the parent pointer if splitting an internal node
				e.child.parent = nn
			}
		}
	}
	//group1
	n.entries = group1
	//group2
	nn.entries = group2

	return nn, nil
}

// chooseSplitAxis() chooses the axis perpendicular to which the split is performed.
func (n *node) chooseSplitAxis() int { //TODO Make the code prettier
	//[CSA 1]
	//Entries sorted by Latitude
	S_lat := 0.000000 //used to determine the best axis to split on
	bestK_lat := 0    //used to determine the best distribution
	minOverlap_lat := -1.000000
	best_area_lat := -1.000000
	sortByLat := make([]entry, len(n.entries)) // len(sortByLat) == len(n.entries) is needed for copy to work
	copy(sortByLat, n.entries)
	sort.Sort(byLat(sortByLat))

	//Entries sorted by Longitude
	S_long := 0.000000 //used to determine the best axis to split on
	bestK_long := 0    //used to determine the best distribution
	minOverlap_long := -1.000000
	best_area_long := -1.000000
	sort.Sort(byLong(n.entries))

	//For each axis: M - 2m + 2 distributions of the M+1 entries into two groups are determined
	d := (RTree_M - (2 * RTree_m) + 2)
	for k := 1; k <= d; k++ {
		//By Latitude
		LatGroup1 := make([]entry, (RTree_m - 1 + k))
		LatGroup2 := make([]entry, (RTree_M - len(LatGroup1) + 1))
		copy(LatGroup1, sortByLat[:RTree_m-1+k])
		copy(LatGroup2, sortByLat[RTree_m-1+k:])
		latGoodness := marginOf(LatGroup1) + marginOf(LatGroup2)
		S_lat += latGoodness
		// test if this distribution has the best overlap value for latitude
		mbr1 := mbrOf(LatGroup1...)
		mbr2 := mbrOf(LatGroup2...)
		if o := mbr1.OverlapWith(mbr2); o <= minOverlap_lat || minOverlap_lat == -1 {
			if o < minOverlap_lat || minOverlap_lat == -1 {
				bestK_lat = k //we have a new best
				minOverlap_lat = o
				best_area_lat = mbr1.Area() + mbr2.Area()
			} else { //tie -> keep the distribution with the least area
				a_now := mbr1.Area() + mbr2.Area()
				if a_now < best_area_lat {
					bestK_lat = k //we have a new best
					minOverlap_lat = o
					best_area_lat = mbr1.Area() + mbr2.Area()
				}
			}
		} //else don't change the value

		//By Longitude
		LongGroup1 := make([]entry, (RTree_m - 1 + k))
		LongGroup2 := make([]entry, (RTree_M - len(LongGroup1) + 1))
		copy(LongGroup1, n.entries[:RTree_m-1+k])
		copy(LongGroup2, n.entries[RTree_m-1+k:])
		longGoodness := marginOf(LongGroup1) + marginOf(LongGroup2)
		S_long += longGoodness
		// test if this distribution has the best overlap value for longitude
		mbr1 = mbrOf(LongGroup1...)
		mbr2 = mbrOf(LongGroup2...)
		if o := mbr1.OverlapWith(mbr2); o <= minOverlap_long || minOverlap_long == -1 {
			if o < minOverlap_long || minOverlap_long == -1 {
				bestK_long = k //we have a new best
				minOverlap_long = o
				best_area_long = mbr1.Area() + mbr2.Area()
			} else { //tie -> keep the distribution with the least area
				a_now := mbr1.Area() + mbr2.Area()
				if a_now < best_area_long {
					bestK_long = k //we have a new best
					minOverlap_long = o
					best_area_long = mbr1.Area() + mbr2.Area()
				}
			}
		} //else don't change the value
	}
	//CSA2: Choose the axis with the minimum S as split axis
	if S_lat < S_long {
		n.entries = sortByLat
		return bestK_lat
	}
	return bestK_long
}

// recalculateMBR returns the MBR that contains all the children of n.
func (n *node) recalculateMBR() *geo.Rectangle {
	return mbrOf(n.entries...)
}

// marginOf returns the margin of the MBR containing the entries.
func marginOf(entries []entry) float64 {
	return mbrOf(entries...).Margin()
}

// mbrOf returns the MBR of some entry-objects.
func mbrOf(entries ...entry) *geo.Rectangle {
	nMinLat := entries[0].mbr.Min().Lat
	nMinLong := entries[0].mbr.Min().Long
	nMaxLat := entries[0].mbr.Max().Lat
	nMaxLong := entries[0].mbr.Max().Long
	for _, e := range entries {
		if e.mbr.Min().Lat < nMinLat {
			nMinLat = e.mbr.Min().Lat
		}
		if e.mbr.Min().Long < nMinLong {
			nMinLong = e.mbr.Min().Long
		}
		if e.mbr.Max().Lat > nMaxLat {
			nMaxLat = e.mbr.Max().Lat
		}
		if e.mbr.Max().Long > nMaxLong {
			nMaxLong = e.mbr.Max().Long
		}
	}
	r, err := geo.NewRectangle(nMinLat, nMinLong, nMaxLat, nMaxLong)

	CheckErr(err, "mbrOf had some trouble creating a new MBR of the provided entries")
	return r
}

// FindWithin returns all the boats that overlaps a given rectangle of the map [0].
func (rt *RTree) FindWithin(r *geo.Rectangle) *[]Match {
	n := rt.root
	matches := []entry{}
	if !n.isLeaf() {
		matches = append(matches, n.searchChildren(r, matches)...)
	} else { //only need to search the root node
		for _, e := range n.entries {
			if geo.Overlaps(e.mbr, r) {
				matches = append(matches, e)
			}
		}
	}
	return rt.toMatches(matches)
}

// searchChildren is the recursive method for finding the nodes whose mbr overlaps the searchBox [0].
func (n *node) searchChildren(searchBox *geo.Rectangle, matches []entry) []entry { //TODO Test performance by searching children concurrently?
	if !n.isLeaf() { //Internal node:
		for _, e := range n.entries {
			if geo.Overlaps(e.mbr, searchBox) {
				matches = e.child.searchChildren(searchBox, matches) //recursively search the child node
			}
		}
	} else { //Leaf node:
		for _, e := range n.entries {
			if geo.Overlaps(e.mbr, searchBox) {
				matches = append(matches, e)
			}
		}
	}
	return matches
}

// Update is used to update the location of a boat that is already stored in the structure.
// It deletes the old entry, and inserts a new entry.
func (rt *RTree) Update(mmsi uint32, oldLat, oldLong, newLat, newLong float64) error {
	// Old coordinates
	oldR, err := geo.NewRectangle(oldLat, oldLong, oldLat, oldLong)
	if err != nil {
		return errors.New("Illegal coordinates, please use <latitude, longitude> coodinates")
	}
	// Deletes the old coordinates
	err = rt.delete(mmsi, oldR)
	if err != nil {
		return err
	}
	// Inserts the new coordinates
	rt.InsertData(newLat, newLong, mmsi)
	return nil
}

// delete removes the Point(zero-area Rectangle) from the RTree [0].
func (rt *RTree) delete(mmsi uint32, r *geo.Rectangle) error {
	//D1 [Find node containing record] (and also the index of the entry)
	l, idx := rt.root.findLeaf(mmsi, r)
	if l != nil && idx >= 0 {
		//D2 [Delete record]
		l.entries = append(l.entries[:idx], l.entries[idx+1:]...)
		//D3 [Propagate changes]
		rt.condenseTree(l)
	} else {
		return errors.New("Failed to delete, could not find the leaf node containing the boat")
	}
	rt.numOfBoats--
	return nil
}

// findLeaf finds the leaf node containing the given rectangle r [0].
func (n *node) findLeaf(mmsi uint32, r *geo.Rectangle) (*node, int) {
	if !n.isLeaf() { //FL1
		for _, e := range n.entries {
			if geo.Overlaps(e.mbr, r) {
				l, idx := e.child.findLeaf(mmsi, r) // Searches childnode
				if l != nil {
					return l, idx // The childnode was the correct leafnode
				}
			}
		}
	} else { //FL2 [Search leaf node for record]
		for idx, ent := range n.entries {
			if geo.Overlaps(ent.mbr, r) && mmsi == ent.mmsi { //locating the exact entry
				return n, idx
			}
		}
	}
	return nil, -1 // no match found
}

// condenseTree is used when an entry has been deleted from n [0].
// It traverses the tree from the node and up to the root and makes the necessary changes to the nodes.
func (rt *RTree) condenseTree(n *node) {
	//CT1 [initialize]
	q := []entry{} // Contains orphaned entries
	for rt.root != n {
		//CT2 [find parent entry]
		p := n.parent
		idx, err := n.parentEntriesIdx()
		CheckErr(err, "Trouble condensing the tree")
		en := p.entries[idx] // the entry containing n
		//CT3 [eliminate under-full node]
		if len(n.entries) < RTree_m {
			p.entries = append(p.entries[:idx], p.entries[idx+1:]...) //[8] remove n from its parent
			q = append(q, en.child.entries...)
		} else {
			//CT4 [Adjust MBR] (if n has not been eliminated)
			en.mbr = n.recalculateMBR()
		}
		n = p // CT5 [Move up one height in tree]
	}
	//CT6 [Re-insert orphaned entries]
	for _, e := range q {
		if e.child != nil { //inserting an internal
			rt.insert(e.child.height+1, e, true) //TODO false or true?
			_, err := e.child.parent.parentEntriesIdx()
			CheckErr(err, "Cannot find parent of re-inserted orphaned internal entry")
		} else { //inserting a leaf entry
			rt.insert(0, e, true) //TODO false or true?
		}
	}
	//D4 [Shorten tree] (if root has only 1 child, promote that child to root)
	if len(rt.root.entries) == 1 && !rt.root.isLeaf() {
		rt.root = rt.root.entries[0].child
		rt.root.parent = nil
		//fmt.Printf("Promoted a child to root, new height is %d\n", rt.root.height)
	}
}

// parentEntriesIdx returns the index of the node in its parent's list of entries.
func (n *node) parentEntriesIdx() (int, error) {
	p := n.parent
	if p != nil {
		for idx, e := range p.entries {
			if e.child == n {
				return idx, nil
			}
		}
	}
	return -1, errors.New("This node is not found in parent's entries")
}

// toMatches returns a slice of Match-objects that can be used to create GeoJSON output
func (rt *RTree) toMatches(matches []entry) *[]Match {
	s := []Match{}
	for _, m := range matches {
		s = append(s, Match{m.mmsi, m.mbr.Max().Lat, m.mbr.Max().Long})
	}
	return &s
}

// CheckErr is a function for checking an error.
// Takes the error and a message as input and does log.Fatalf() if error.
func CheckErr(err error, message string) {
	if err != nil {
		log.Fatalf("ERROR: %s \n %s", message, err)
	}
}

/*
TODOs:
    - 180 meridianen... (~International date line)

References:
    [0]        http://www.cs.jhu.edu/%7Emisha/ReadingSeminar/Papers/Guttman84.pdf
    [1]        https://en.wikipedia.org/wiki/Tree_%28data_structure%29
    https://en.wikipedia.org/wiki/R-tree
    https://www.youtube.com/watch?v=39GuS7c4uZI
    https://blog.golang.org/go-slices-usage-and-internals
    https://blog.golang.org/go-maps-in-action
    [7]     http://stackoverflow.com/questions/1760757/how-to-efficiently-concatenate-strings-in-go        http://herman.asia/efficient-string-concatenation-in-go
    [8]        http://stackoverflow.com/questions/25025409/delete-element-in-a-slice
    [9]        http://dbs.mathematik.uni-marburg.de/publications/myPapers/1990/BKSS90.pdf                        (R* Trees)
    [10]     https://en.wikipedia.org/wiki/R*_tree
    [11]    https://golang.org/pkg/sort/
    [12]    http://www.eng.auburn.edu/~weishinn/Comp7970/Presentation/rstartree.pdf
    https://golang.org/ref/spec#Passing_arguments_to_..._parameters
    [13]    http://geojsonlint.com/
    [14]    http://stackoverflow.com/questions/7933460/how-do-you-write-multiline-strings-in-go#7933487
*/
