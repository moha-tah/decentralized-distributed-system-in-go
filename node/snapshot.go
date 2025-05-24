package node

//💡 Implements distributed system snapshots (based on Chandy-Lamport algorithm) to capture 
// consistent global states. Records node states, in-transit messages, and vector clocks for causal consistency.

import (
	"distributed_system/consts"
	"distributed_system/format"
	"distributed_system/utils"
	"strings"
)

type SnapshotData struct {
	VectorClock 	[]int
	Content     	string
	Initiator   	string
	BufferedMsg 	[]string
	NodeName	string
}

type GlobalSnapshot struct {
	SnapshotId 	string
	VectorClock	[]int
	Initiator  	string
	Data 		[]SnapshotData
}


var snap_fieldsep = consts.Snap_fieldsep
var snap_keyvalsep = consts.Snap_keyvalsep
var snap_bfmssage_fieldsep = consts.Snap_bfmssage_fieldsep
var snap_bfmssage_keyvalsep = consts.Fieldsep


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
		for idx, keyval := range  keyvals {
			if keyval == "" {
				continue
			}
			keyval_parts := strings.Split(keyval[1:], consts.Keyvalsep)
			snap_field += keyval_parts[0] + consts.Snap_bfmssage_keyvalsep + keyval_parts[1]
			if idx != len(keyvals) -1 {
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
	initiator := format.Findval(snap_field, "snap_initiator", "DSData")
	vectorClock := format.Findval(snap_field, "snap_vclk", "DSData")
	content := format.Findval(snap_field, "snap_content", "DSData")
	nodeName := format.Findval(snap_field, "snap_node_name", "DSData")
	var bufferedMsgs []string 
	bfMsgField := format.Findval(snap_field, "snap_buffered_msgs", "DSData")
	for _, bfm_msg := range strings.Split(bfMsgField, consts.Snap_bfmssage_fieldsep + consts.Snap_bfmssage_keyvalsep) {
		current_msg := consts.Fieldsep + consts.Keyvalsep
		keyval_parts := strings.Split(bfm_msg, consts.Snap_bfmssage_fieldsep)
		for idx, keyval_part := range keyval_parts {
			if keyval_part == "" {
				continue
			}
			keyval := strings.Split(keyval_part, consts.Snap_bfmssage_keyvalsep)
			current_msg += keyval[0] + consts.Keyvalsep + keyval[1]
			if idx != len(keyval_parts) -1 {
				current_msg += consts.Fieldsep + consts.Keyvalsep
			}
		}
		if current_msg != consts.Fieldsep + consts.Keyvalsep {
			bufferedMsgs = append(bufferedMsgs, current_msg)
		}
	}

	vc, err := utils.DeserializeVectorClock(vectorClock)
	if err != nil {
		format.Display(format.Format_e("DSData", "DSData", "Error with deserializing VC: "+err.Error()))
	}
	return SnapshotData{
		Initiator: initiator,
		VectorClock: vc,
		Content: content,
		BufferedMsg: bufferedMsgs,
		NodeName: nodeName,
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
	initiator := format.Findval(snap_field, "snap_initiator", "DSData")
	vectorClock := format.Findval(snap_field, "snap_vclk", "DSData")
	snapshotId := format.Findval(snap_field, "snap_id", "DSData")
	var data []SnapshotData

	// The data field is from "snap_data#" to the next "{{":
	start_index := strings.Index(snap_field, "snap_data#") + len("snap_data#") + 2
	if start_index == -1 {
		format.Display(format.Format_e("DSData", "DSData", "Error with deserializing GlobalSnapshot: no data field"))
	}
	sdataField := snap_field[start_index:]
	
	for _, sdata := range strings.Split(sdataField, consts.Snap_bfmssage_fieldsep + consts.Snap_bfmssage_fieldsep) {
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
		SnapshotId: snapshotId,
		VectorClock: vc,
		Initiator: initiator,
		Data: data,
	}
}
