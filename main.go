package main

import (
	"distributed_system/node"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {

	node_type := flag.String("node_type", "sensor", "Type of node")
	// node_name := flag.String("node_name", "Sensor 1", "Name of the node")
	node_id := flag.String("node_id", "0", "Unique ID of the node")

	flag.Parse()

	var child_node node.Node = nil

	switch *node_type {
	case "sensor":
		// node.Sensor(node_name)

		interval := time.Duration(2) * time.Second
		errorRate := 0.1

		child_node = node.NewSensorNode(*node_id, interval, errorRate)
	case "verifier":
		processingCapacity := 1 // Number of readings to process at once
		threshold := 10.0       // Temperature deviation threshold
		child_node = node.NewVerifierNode(*node_id, processingCapacity, threshold)
		// node.Start()
	case "user":
		predictionWindow := 24 * time.Hour
		child_node = node.NewUserNode(*node_id, "placeholder", predictionWindow)
		// node.Start()
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid node_type %q. Must be 'sensor (default)', 'verifier', or 'user'.\n", *node_type)
		os.Exit(1) // Exit with error
	}

	control_layer := node.NewControlLayer(*node_id+"_control", child_node)

	control_layer.Start()
	// child_node.Start()

	// Block forever or until signal
	select {} // empty select blocks forever
}
