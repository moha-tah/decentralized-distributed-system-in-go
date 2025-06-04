package main

import (
	"distributed_system/node"
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <node_type> <node_id> [peer1:port peer2:port ...]")
		return
	}

	node_type := os.Args[1] 	// Node type (e.g., "client", "server")
	node_id := os.Args[2]  		// Node ID (should be an integer)
	peers := os.Args[3:]		// Optional list of peer addresses (e.g., "peer1:port", "peer2:port")

	// Check that node_type is valid
	node_id_int, err := strconv.Atoi(node_id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid node_id %q. Must be an integer.\n", node_id)
		os.Exit(1) // Exit with error
	}

	// Check that nb of peers is valid (0 or 2)
	if len(peers) > 2 {
		fmt.Fprintf(os.Stderr, "Error: Peers not valid. Number of peers must be 0 or 2 peers (or 1 for ring creation).\n")
		os.Exit(1) // Exit with error
	}
	if len(peers) == 2 {
		seen := make(map[string]bool)
			for _, item := range peers {
			if seen[item] {
				fmt.Fprintf(os.Stderr, "Error: Duplicate peer address %q found in the list of peers.\n", item)
				os.Exit(1) // Exit with error
			}
			seen[item] = true
		}
	}

	listenPort := strconv.Itoa(9000 + node_id_int) // port calculation based on node_id

	var child_node node.Node = nil


	baseTempLow := float32(15.0)
	baseTempHigh := float32(30.0)

	basePort := 8080

	switch node_type {
	case "sensor":
		interval := time.Duration(1) * time.Second
		errorRate := float32(0.5)
		child_node = node.NewSensorNode(node_id, interval, errorRate, baseTempLow, baseTempHigh)
	case "verifier":
		processingCapacity := 1   // Number of readings to process at once
		threshold := float32(2.0) // Temperature deviation threshold
		child_node = node.NewVerifierNode(node_id, processingCapacity, threshold, baseTempLow, baseTempHigh)
	case "user_exp":
		child_node = node.NewUserNode(node_id, "exp", basePort+node_id_int)
	case "user_linear":
		child_node = node.NewUserNode(node_id, "linear", basePort+node_id_int)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid node_type %q. Must be 'sensor (default)', 'verifier', or 'user_exp' or `user_linear`.\n", node_type)
		os.Exit(1) // Exit with error
	}

	control_layer := node.NewControlLayer(node_id+"_control", child_node)
	network_layer := node.NewNetworkLayer(node_id, node_type, &child_node, control_layer, listenPort, peers)

	network_layer.Start() // Start the network layer

	// Block forever or until signal
	select {} // empty select blocks forever
}


// This function is used to demonstrate the serialization and deserialization of snapshot data.
// It creates a snapshot data object, serializes it to a string, and then deserializes it back to an object.
// It also creates a global snapshot object, serializes it, and deserializes it back to an object.
// Used to verify the serialization and deserialization functions.
func demoSnashotSerialization() {
	sdata := node.SnapshotData{
		VectorClock: []int{0, 0, 1},
		Initiator:   "node0",
		BufferedMsg: []string{
			"/=clk=1/=content_type=siteName/=content_value=control (5_control)",
			"/=clk=2/=content_type=siteName2/=content_value=control (2_control)",
		},
		Content: "contentmsg?",
	}
	sdata_s := node.SerializeSnapshotData(sdata)
	fmt.Println(sdata_s)

	fmt.Println("=================== Deserialized SnapshotData ====================")
	sdata_r := node.DeserializeSnapshotData(sdata_s)
	fmt.Println(sdata_r.Initiator)
	fmt.Println(sdata.Content)
	for _, m := range sdata_r.BufferedMsg {
		fmt.Println(m)
	}

	sdata2 := node.SnapshotData{
		VectorClock: []int{0, 1, 1},
		Initiator:   "node1",
		BufferedMsg: []string{
			"/=clk=3/=content_type=siteName/=content_value=control (3_control)",
			"/=clk=4/=content_type=siteName2/=content_value=control (4_control)",
		},
		Content: "contentmsg3333",
	}

	fmt.Println("\n=================== Serialized GlobalSnapshot ====================")

	global_data := node.GlobalSnapshot{
		VectorClock: []int{1, 1, 1},
		Initiator:   "node0",
		Data:        []node.SnapshotData{sdata, sdata2},
	}
	global_data_s := node.SerializeGlobalSnapshot(global_data)
	fmt.Println(global_data_s)
	fmt.Println("=================== Deserialized GlobalSnapshot ====================")
	global_data_r := node.DeserializeGlobalSnapshot(global_data_s)
	fmt.Println(global_data_r.Initiator)
	for i := 0; i < len(global_data_r.Data); i++ {
		fmt.Println(global_data_r.Data[i].Content)
		fmt.Println(global_data_r.Data[i].BufferedMsg[0])
		fmt.Println(global_data_r.Data[i].BufferedMsg[1])
		fmt.Println(global_data_r.Data[i].Initiator)
	}

	return
}
