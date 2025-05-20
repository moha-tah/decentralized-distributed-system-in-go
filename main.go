package main

import (
	"distributed_system/node"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {

	node_type := flag.String("node_type", "sensor", "Type of node")
	node_id := flag.String("node_id", "0", "Unique ID of the node")
	flag.Parse()

	var child_node node.Node = nil

	node_id_int, err := strconv.Atoi(*node_id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid node_id %q. Must be an integer.\n", *node_id)
		os.Exit(1) // Exit with error
	}

	baseTempLow := float32(15.0)
	baseTempHigh := float32(30.0)

	basePort := 8080

	switch *node_type {
	case "sensor":
		interval := time.Duration(2) * time.Second
		errorRate := float32(0.5)
		child_node = node.NewSensorNode(*node_id, interval, errorRate, baseTempLow, baseTempHigh)
	case "verifier":
		processingCapacity := 1 // Number of readings to process at once
		threshold := float32(2.0)       // Temperature deviation threshold
		child_node = node.NewVerifierNode(*node_id, processingCapacity, threshold, baseTempLow, baseTempHigh)
	case "user_exp":
		child_node = node.NewUserNode(*node_id, "exp", basePort+node_id_int)
	case "user_linear":
		child_node = node.NewUserNode(*node_id, "linear", basePort+node_id_int)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid node_type %q. Must be 'sensor (default)', 'verifier', or 'user_exp' or `user_linear`.\n", *node_type)
		os.Exit(1) // Exit with error
	}

	control_layer := node.NewControlLayer(*node_id+"_control", child_node)
	control_layer.Start()

	// Block forever or until signal
	select {} // empty select blocks forever
}
