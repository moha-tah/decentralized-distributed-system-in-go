package utils
//
// import (
// 	"fmt"
// 	"strconv"
// 	"strings"
// )
//
// // MID = Message ID
// // Is it the int part of a message for a node,
// // meaning the `nbMsgSent` of a node.
// // /!\ It MIGHT be something more complex than an int!
// // This is why a dedicated struct is already in place.
//
//
// // A MID is a int
// type MID struct {
//     V int
// }
//
// // A MIDPair is used to define the bounds of 
// // known messages intervals.
// type MIDPair struct {
//     Lower MID
//     Upper MID
// }
//
// type MIDPairIntervals struct {
//     Intervals []MIDPair
// }
//
// type MIDWatcher struct {
// 	site_clock map[string]MIDPairIntervals
// }
//
// // ============= MID =================
//
// // LessThan says is a clock is smaller (strictly) than another.
// func (c MID) LessThan(other MID) bool {
// 	return c.V < other.V
// }
//
// func (c MID) LessThanOrEqual(other MID) bool {
//     return c.LessThan(other) || c.Equal(other)
// }
//
// func (c MID) Equal(other MID) bool {
// 	return c.V == other.V
// }
//
// // Is clock c exactly one “step” above other?
// // I.e. all components either equal, except exactly one that is +1.
// func (c MID) IsAdjacentAbove(other MID) bool {
// 	return c.V == other.V + 1
// }
//
// // Is c exactly one “step” below other?
// func (c MID) IsAdjacentBelow(other MID) bool {
// 	return c.V == other.V - 1
// }
//
// // String converts a clock to a string 
// // in order to send it through messages.
// func (c MID) String() string {
// 	return strconv.Itoa(c.V)
// }
//
// // Example usage: 
// // clockStr := "(0, 0, 1)"
// // c, err := MIDFromString(clockStr)
// // if err != nil {
// //     fmt.Println("Error:", err)
// // } else {
// //     fmt.Println("Parsed MID:", c.String())
// // }
// func MIDFromString(s string) (MID, error) {
// 	v, err := strconv.Atoi(s)
//         if err != nil {
//             return MID{}, fmt.Errorf("invalid integer: %v", err)
//         }
//     	return MID{V: v}, nil
// }
//
//
//
// // ============= MIDPairInvtervals =================
//
// // Instanciate a new MIDPairIntervals
// func NewMIDPairIntervals() *MIDPairIntervals {
//     return &MIDPairIntervals{Intervals: make([]MIDPair, 0)}
// }
//
// // Add a MIDPair to a given cpi
// func (cpi *MIDPairIntervals) AddPair(pair MIDPair) {
//     cpi.Intervals = append(cpi.Intervals, pair)
// }
//
//
// // Check wether a clock is contained in one of the MIDPair
// func (cpi *MIDPairIntervals) Contains(clock MID) bool {
//     for _, pair := range cpi.Intervals {
//         if pair.Lower.LessThanOrEqual(clock) && clock.LessThanOrEqual(pair.Upper) {
//             return true
//         }
//     }
//     return false
// }
//
// // AddMID implements the four "behaviors" of adding a clock:
// // - if the clock is stricly contained in on pair, do nothing
// // - if the clock is just one above or one lower than a pair's bound 
// //	=> update the pair to match the clock
// // - if clock is very different
// // 	=> create another clockpair with the same clock for lower and upper
// // - it the clock is the mergeing point of two pairs
// // 	=> create one pair out of them (with the lowest lower bound and highest upper bound)
// func (cpi *MIDPairIntervals) AddMID(clk MID) {
//     var (
//         idxGrowLow  = -1 // index where clk is one below Lower
//         idxGrowHigh = -1 // index where clk is one above Upper
//     )
//
//     for i, p := range cpi.Intervals {
//         // already inside this interval?
//         if p.Lower.LessThanOrEqual(clk) && clk.LessThanOrEqual(p.Upper) {
//             return // nothing to do
//         }
//         // exactly one step below this interval’s lower?
//         if clk.IsAdjacentBelow(p.Lower) {
//             idxGrowLow = i
//         }
//         // exactly one step above this interval’s upper?
//         if clk.IsAdjacentAbove(p.Upper) {
//             idxGrowHigh = i
//         }
//     }
//
//     // 1) Bridge two intervals? merge them.
//     if idxGrowLow != -1 && idxGrowHigh != -1 && idxGrowLow != idxGrowHigh {
//         i, j := idxGrowLow, idxGrowHigh
//         if j < i {
//             i, j = j, i
//         }
//         low  := cpi.Intervals[i].Lower
//         high := cpi.Intervals[j].Upper
//         // remove both and append merged
//         merged := MIDPair{Lower: low, Upper: high}
//         newInts := append(cpi.Intervals[:i], cpi.Intervals[i+1:j]...)
//         newInts = append(newInts, cpi.Intervals[j+1:]...)
//         cpi.Intervals = append(newInts, merged)
//         return
//     }
//
//     // 2) Grow an existing interval’s lower bound down
//     if idxGrowLow != -1 {
//         cpi.Intervals[idxGrowLow].Lower = clk
//         return
//     }
//
//     // 3) Grow an existing interval’s upper bound up
//     if idxGrowHigh != -1 {
//         cpi.Intervals[idxGrowHigh].Upper = clk
//         return
//     }
//
//     // 4) No adjacency, no containment → new singleton interval
//     cpi.Intervals = append(cpi.Intervals, MIDPair{Lower: clk, Upper: clk})
// }
//
//
// // ============ MIDWatcher ======== 
//
// func NewMIDWatcher() *MIDWatcher {
// 	return &MIDWatcher{site_clock: make(map[string]MIDPairIntervals)}
// }
//
// // Find the node by name (or false if not in list)
// // and check if the clock is contained in one of its interval.
// func (cw *MIDWatcher) ContainsMID(nodeID string, clk MID) bool {
// 	cpi, exists := cw.site_clock[nodeID]
// 	if !exists {
// 		return false
// 	}
// 	return cpi.Contains(clk)
// }
//
//
// // Add the clock to given site (create site in map if 
// // not already in list).
// func (cw *MIDWatcher) AddMIDToNode(nodeID string, clk MID) {
// 	cpi, exists := cw.site_clock[nodeID]
// 	if !exists {
// 		cpi = *NewMIDPairIntervals()
// 	}
// 	cpi.AddMID(clk)
// 	cw.site_clock[nodeID] = cpi
// 	// Debug:
// }
//
// // String representation of MIDWatcher
// func (cw *MIDWatcher) String() string {
//     var sb strings.Builder
//     for nodeID, intervals := range cw.site_clock {
//         sb.WriteString(fmt.Sprintf("Node: %s\n", nodeID))
//         for _, pair := range intervals.Intervals {
//             sb.WriteString(fmt.Sprintf("  Interval: [%v] – [%v]\n", pair.Lower.V, pair.Upper.V))
//         }
//     }
//     return sb.String()
// }
