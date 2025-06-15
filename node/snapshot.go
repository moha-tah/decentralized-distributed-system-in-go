package node

//💡 Implements distributed system snapshots (based on Chandy-Lamport algorithm) to capture
// consistent global states. Records node states, in-transit messages, and vector clocks for causal consistency.

import (
	"distributed_system/consts"
	"distributed_system/format"
	"distributed_system/utils"
	"encoding/csv"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type SnapshotData struct {
	VectorClock []int
	Content     string
	Initiator   string
	BufferedMsg []string
	NodeName    string
}

type GlobalSnapshot struct {
	SnapshotId  string
	VectorClock []int
	Initiator   string
	Data        []SnapshotData
}

var snap_fieldsep = consts.Snap_fieldsep
var snap_keyvalsep = consts.Snap_keyvalsep


func (n* NetworkLayer) AskSnapshotControlAndPropagateSnapshotRequest(msg string, msg_received_from_id string) {
	// Ask the control layer to take a snapshot 
	next_message := n.controlLayer.TakeSnapshotAndPrepareMessage(msg)
	if next_message != "" { // is "" if the our control layer is the initiator
		for _, neighbor := range n.activeNeighborsIDs {
			if strconv.Itoa(neighbor) != msg_received_from_id { // Don't send it back to the sender
				n.SendMessage(next_message, neighbor)
				return
			}
		}
	}
}

// ⚠️ FIFO hypothesis
func (c *ControlLayer) TakeSnapshotAndPrepareMessage(msg string) string{
	msg_type := format.Findval(msg, "type")
	initiator := format.Findval(msg, "snapshot_initiator")
	format.Display_g("TakeSnapshotAndPrepareMessage()", c.GetName(), "Received snapshot request from "+initiator+" my ID is "+c.id)

	snapshot_id, snap_err := strconv.Atoi(format.Findval(msg, "snapshot_id"))
	if snap_err != nil {
		format.Display_e("handleSnapshotMsg()", c.GetName(), "Error parsing snapshot_id: "+snap_err.Error())
	}

	//⚠️ FIFO hypothesis: if a node receives a snapshot request, it has already
	// received a marker before from the same node.
	if msg_type == "snapshot" {
		state_str := format.Findval(msg, "content_value")
		state := DeserializeGlobalSnapshot(state_str)

		if c.id == format.Findval(msg, "snapshot_initiator_id") {
			format.Display_g("handleSnapshotMsg()", c.GetName(), "Initiator received its own snapshot request")
			snapshots := make(map[string]SnapshotData)
			for _, state_data := range state.Data {
				snapshots[state_data.NodeName] = state_data
			}
			c.SaveSnapshotToCSVThreadSafe(snapshots, format.Findval(msg, "snapshot_id"))
			c.child.SetSnapshotInProgress(false) // For the user to update its UI
		} else {
			snap_data := c.takeSnapshot(snapshot_id, initiator)

			c.mu.Lock() // Update the SubTreeState with the received state
			subTreeState := c.subTreeState
			subTreeState.Data = []SnapshotData{snap_data}
			subTreeState.Data = append(subTreeState.Data, state.Data...)
			c.subTreeState = subTreeState
			c.mu.Unlock()

			response := format.Msg_format_multi(format.Build_msg_args(
				"id", c.GenerateUniqueMessageID(),
				"type", "snapshot",
				"sender_name", c.GetName(),
				"sender_name_source", c.GetName(),
				"sender_type", "control",
				"content_type", "snapshot_data",
				"content_value", SerializeGlobalSnapshot(c.subTreeState),
				"snapshot_initiator", initiator,
				"snapshot_initiator_id", format.Findval(msg, "snapshot_initiator_id"),
				"snapshot_id", strconv.Itoa(snapshot_id),
				"propagation", "true",
				))
			c.mu.Lock()
			c.nbMsgSent++
			c.mu.Unlock()
			// c.SendMsgToNetwork(response) // Let network layer handle the snapshot propagation.
			return response
		}
			
	}  
	return ""
}

func (c *ControlLayer) takeSnapshot(snapshot_id int, initiator string) SnapshotData {
	c.mu.Lock()
	c.snapResponseSent = false
	c.nbSnapResponsePending = len(c.childrenNames)
	state := SnapshotData{
		VectorClock: c.vectorClock,
		Content:     c.child.GetLocalState(),
		Initiator:   initiator,
		NodeName:    c.GetName(),
		BufferedMsg: make([]string, 0),
	}

	subTreeState := GlobalSnapshot{} // Reset current snapshot
	if subTreeState.Initiator == "" {
		subTreeState.Initiator = state.Initiator
	}
	if subTreeState.Data == nil {
		subTreeState.Data = []SnapshotData{}
	}
	orig := c.vectorClock
	copyOfVC := make([]int, len(orig))
	copy(copyOfVC, orig)
	subTreeState.VectorClock = copyOfVC
	subTreeState.Data = append(subTreeState.Data, state)
	subTreeState.SnapshotId = strconv.Itoa(snapshot_id)
	c.subTreeState = subTreeState

	c.markersReceivedFrom[c.nodeMarker] = make([]string, 0)
	c.markersReceivedFrom[c.nodeMarker] = append(c.markersReceivedFrom[c.nodeMarker], c.GetName())
	c.nodeMarker = snapshot_id

	c.mu.Unlock()
	return state
}

// RequestSnapshot sends a snapshot request to all neighbors.
// It first takes a snapshot and then sends the request to other nodes.
// As it first takes a snapshot, nodes will first receive the
// marker message from `takeSnapshotAndPropagate()` and then the request message.
func (c *ControlLayer) RequestSnapshot() {
	c.mu.Lock()
	snapshot_id := c.nodeMarker + 1
	c.mu.Unlock()

	snap_data := c.takeSnapshot(snapshot_id, c.GetName())

	global_snapshot := GlobalSnapshot{
		SnapshotId:  strconv.Itoa(snapshot_id),
		VectorClock: c.vectorClock,
		Initiator:   c.GetName(),
		Data:        []SnapshotData{snap_data},
	}

	msgID := c.GenerateUniqueMessageID()
	msg := format.Msg_format_multi(format.Build_msg_args(
		"id", msgID,
		"type", "snapshot",
		"sender_name", c.GetName(),
		"sender_name_source", c.GetName(),
		"sender_type", "control",
		"content_type", "snapshot_data",
		"content_value", SerializeGlobalSnapshot(global_snapshot),
		"propagation", "true",
		"snapshot_initiator", c.GetName(),
		"snapshot_initiator_id", c.id,
		"snapshot_id", strconv.Itoa(snapshot_id),
	))
	c.mu.Lock()
	c.nbMsgSent++
	c.mu.Unlock()
	c.SendMsgToNetwork(msg) // Let network layer handle the request.
	// for _, neighbor := range c.directNeighbors {
	// 	c.SendMsg(format.Replaceval(msg, "destination", neighbor))
	// }
}

// Converts SnapshotData struct to string format using custom delimiters.
// Handles vector clocks and buffered messages serialization.
func SerializeSnapshotData(sdata SnapshotData) string {
	snap_field := ""
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_initiator" + snap_keyvalsep + sdata.Initiator
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_node_name" + snap_keyvalsep + sdata.NodeName
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_vclk" + snap_keyvalsep + utils.SerializeVectorClock(sdata.VectorClock)
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_content" + snap_keyvalsep + sdata.Content
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_buffered_msgs" + snap_keyvalsep
	for _, bf_msg := range sdata.BufferedMsg {
		snap_field += consts.Snap_bfmssage_fieldsep + consts.Snap_bfmssage_keyvalsep
		keyvals := strings.Split(bf_msg, consts.Fieldsep)
		for idx, keyval := range keyvals {
			if keyval == "" {
				continue
			}
			keyval_parts := strings.Split(keyval[1:], consts.Keyvalsep)
			snap_field += keyval_parts[0] + consts.Snap_bfmssage_keyvalsep + keyval_parts[1]
			if idx != len(keyvals)-1 {
				snap_field += consts.Snap_bfmssage_fieldsep
			}
			// for _, m := range keyval_parts {
			// 	fmt.Println(m)
			// }
			// snap_field +=  consts.Snap_bfmssage_fieldsep + keyval_parts[0] + consts.Snap_bfmssage_keyvalsep + keyval_parts[1]
		}
	}
	return snap_field
}

// Reconstructs SnapshotData from serialized string.
// Parses fields and rebuilds buffered messages with proper formatting.
func DeserializeSnapshotData(snap_field string) SnapshotData {
	initiator := format.Findval(snap_field, "snap_initiator")
	vectorClock := format.Findval(snap_field, "snap_vclk")
	content := format.Findval(snap_field, "snap_content")
	nodeName := format.Findval(snap_field, "snap_node_name")
	var bufferedMsgs []string
	bfMsgField := format.Findval(snap_field, "snap_buffered_msgs")
	for _, bfm_msg := range strings.Split(bfMsgField, consts.Snap_bfmssage_fieldsep+consts.Snap_bfmssage_keyvalsep) {
		current_msg := consts.Fieldsep + consts.Keyvalsep
		keyval_parts := strings.Split(bfm_msg, consts.Snap_bfmssage_fieldsep)
		for idx, keyval_part := range keyval_parts {
			if keyval_part == "" {
				continue
			}
			keyval := strings.Split(keyval_part, consts.Snap_bfmssage_keyvalsep)
			current_msg += keyval[0] + consts.Keyvalsep + keyval[1]
			if idx != len(keyval_parts)-1 {
				current_msg += consts.Fieldsep + consts.Keyvalsep
			}
		}
		if current_msg != consts.Fieldsep+consts.Keyvalsep {
			bufferedMsgs = append(bufferedMsgs, current_msg)
		}
	}

	vc, err := utils.DeserializeVectorClock(vectorClock)
	if err != nil {
		format.Display(format.Format_e("DSData", "DSData", "Error with deserializing VC: "+err.Error()))
	}
	return SnapshotData{
		Initiator:   initiator,
		VectorClock: vc,
		Content:     content,
		BufferedMsg: bufferedMsgs,
		NodeName:    nodeName,
	}

}

// Converts GlobalSnapshot to string. Serializes metadata first, then all node snapshots.
// Note: snap_data field must be last.
func SerializeGlobalSnapshot(gdata GlobalSnapshot) string {
	snap_field := snap_fieldsep + snap_keyvalsep + "snap_id" + snap_keyvalsep + gdata.SnapshotId
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_initiator" + snap_keyvalsep + gdata.Initiator
	snap_field += snap_fieldsep + snap_keyvalsep + "snap_vclk" + snap_keyvalsep + utils.SerializeVectorClock(gdata.VectorClock)

	//🔥 The "snap_data" field MUST BE AT THE END (no other fields after it)
	if len(gdata.Data) > 0 {
		snap_field += snap_fieldsep + snap_keyvalsep + "snap_data" + snap_keyvalsep

		for _, sdata := range gdata.Data {
			snap_field += consts.Snap_bfmssage_fieldsep + consts.Snap_bfmssage_fieldsep + SerializeSnapshotData(sdata)
		}
	}
	return snap_field
}

// Reconstructs GlobalSnapshot from string. Extracts metadata, locates data section,
// and deserializes individual node snapshots.
func DeserializeGlobalSnapshot(snap_field string) GlobalSnapshot {
	initiator := format.Findval(snap_field, "snap_initiator")
	vectorClock := format.Findval(snap_field, "snap_vclk")
	snapshotId := format.Findval(snap_field, "snap_id")
	var data []SnapshotData

	// The data field is from "snap_data#" to the next "{{":
	start_index := strings.Index(snap_field, "snap_data#") + len("snap_data#") + 2
	if start_index == -1 {
		format.Display(format.Format_e("DSData", "DSData", "Error with deserializing GlobalSnapshot: no data field"))
	}
	sdataField := snap_field[start_index:]

	for _, sdata := range strings.Split(sdataField, consts.Snap_bfmssage_fieldsep+consts.Snap_bfmssage_fieldsep) {
		if sdata == "" {
			continue
		}
		data = append(data, DeserializeSnapshotData(sdata))
	}
	vc, err := utils.DeserializeVectorClock(vectorClock)
	if err != nil {
		format.Display(format.Format_e("DSData", "DSData", "Error with deserializing VC: "+err.Error()))
	}
	return GlobalSnapshot{
		SnapshotId:  snapshotId,
		VectorClock: vc,
		Initiator:   initiator,
		Data:        data,
	}
}

// CheckConsistency scans through all collected SnapshotData,
// builds a map from nodeID → vectorClock, and then verifies
//
//	for all i,j:  vcMap[i][j] ≤ vcMap[j][j].
//
// If any check fails, the cut is inconsistent.
func (c *ControlLayer) CheckConsistency() bool {
	c.mu.Lock()
	// 1) Build a map[int][]int where the key is nodeID, and the value is that node's VC slice.
	vcMap := make(map[int][]int)

	// Use a regex to extract the integer ID out of NodeName, which has form "control (3_control)" etc.
	re := regexp.MustCompile(`\((\d+)_`)

	for _, sd := range c.subTreeState.Data {
		// Extract the node ID as an integer
		m := re.FindStringSubmatch(sd.NodeName)
		if m == nil {
			format.Display(format.Format_e(c.GetName(), "CheckConsistency()", "Could not parse node ID from NodeName: "+sd.NodeName))
			return false
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			format.Display(format.Format_e(c.GetName(), "CheckConsistency()", "Error parsing node ID from NodeName: "+sd.NodeName+" - "+err.Error()))
			return false
		}

		// Store the vector clock slice
		vcMap[id] = sd.VectorClock
	}

	// 2) If we have M nodes in the map, we expect each VC slice to have length ≥ M.
	//    => Find the maximum nodeID to know how many positions to check.
	maxID := -1
	for id := range vcMap {
		if id > maxID {
			maxID = id
		}
	}
	N := maxID + 1

	// 3) Verify that every vector‐clock slice is at least length N
	for id, vc := range vcMap {
		if len(vc) < N {
			format.Display(format.Format_w(c.GetName(), "CheckConsistency()",
				"⚠️  VectorClock for node "+strconv.Itoa(id)+" has length "+strconv.Itoa(len(vc))+", but expected at least "+strconv.Itoa(N)+" entries"))
			return false
		}
	}

	// 4) Now check the consistency condition:
	//    For every pair (i, j), we must have vcMap[i][j] ≤ vcMap[j][j].
	consistent := true
	for i, vci := range vcMap {
		for j := 0; j < N; j++ {
			// If node j wasn't present in vcMap, that is already wrong.
			vcj, exists := vcMap[j]
			if !exists {
				format.Display(format.Format_w(c.GetName(), "CheckConsistency()",
					"⚠️  Missing snapshot data for node "+strconv.Itoa(j)))
				return false
			}
			if vci[j] > vcj[j] {
				format.Display(format.Format_w(c.GetName(), "CheckConsistency()",
					"⚠️  Inconsistency detected: VC["+strconv.Itoa(i)+"]["+strconv.Itoa(j)+"] = "+strconv.Itoa(vci[j])+
						"  >  VC["+strconv.Itoa(j)+"]["+strconv.Itoa(j)+"] = "+strconv.Itoa(vcj[j])))
				consistent = false
			}
		}
	}

	if consistent {
		format.Display(format.Format_g(c.GetName(), "CheckConsistency()", "✅  The global cut is consistent (all VC[i][j] ≤ VC[j][j])."))
	} else {
		format.Display(format.Format_w(c.GetName(), "CheckConsistency()", "⚠️  The global cut is NOT consistent (some VC[i][j] > VC[j][j])."))
	}
	c.mu.Unlock()
	return consistent
}

func (c *ControlLayer) SaveSnapshotToCSVThreadSafe(snapshots map[string]SnapshotData, snapshotId string) {
	var file *os.File
	var err error

	if snapshotId == "0" {
		// Overwrite the file
		file, err = os.Create("snapshot.csv")
	} else {
		// Append to the file
		file, err = os.OpenFile("snapshot.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	
	if err != nil {
		log.Println("Erreur ouverture fichier snapshot:", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if snapshotId == "0" {
		// En-tête détaillé
		writer.Write([]string{
			"Snapshot ID",
			"NodeName",
			"Initiator",
			"VectorClock",
			"Content",
			"BufferedMessages",
		})
	}

	for _, snap := range snapshots {
		// Concatène les messages bufferisés en une seule chaîne
		buffered := ""
		if len(snap.BufferedMsg) > 0 {
			buffered = strings.Join(snap.BufferedMsg, " || ")
		}
		writer.Write([]string{
			snapshotId,
			snap.NodeName,
			snap.Initiator,
			utils.SerializeVectorClock(snap.VectorClock),
			snap.Content,
			buffered,
		})
	}
}
