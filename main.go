package main

import (
	"fmt"
	"distributed_system/node"
	"flag"
	"os"
)

func main() {

	node_type := flag.String("node_type", "sensor", "Type of node")
	node_name := flag.String("node_name", "Sensor 1", "Name of the node")
	flag.Parse()

	if *node_type != "sensor" && *node_type != "verifier" && *node_type != "user" {
		fmt.Fprintf(os.Stderr, "Error: invalid node_type %q. Must be 'sensor (default)', 'verifier', or 'user'.\n", *node_type)
		os.Exit(1) // Exit with error	
	}

	if *node_type == "sensor" {
		node.Sensor(node_name)
	} else if *node_type == "verifier" {
		node.Verifier(node_name)
	} else if *node_type == "user" {
		node.User(node_name)
	}

}
