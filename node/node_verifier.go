package node

import (
	"strconv"
	// "bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	// "os"    // Use for the bufio reader: reads from os stdin
	// "strings"
	"distributed_system/format"
	"distributed_system/utils"
	"log" // Added for logging errors
)

// VerifierNode represents a verifier in the system
type VerifierNode struct {
	BaseNode
	// store              *storage.DataStore
	processingCapacity int
	threshold          float64
	verificationLocks  map[string]bool            // Maps day IDs to lock status
	lockRequests       map[string]map[string]int  // Maps day IDs to node IDs and timestamps
	lockReplies        map[string]map[string]bool // Maps day IDs to node IDs and reply status
	otherVerifiers     map[string]bool            // Set of other verifier node IDs
	recentReadings     []float32                  // FIFO queue for the last readings
}

// NewVerifierNode creates a new verifier node
func NewVerifierNode(id string, capacity int, threshold float64) *VerifierNode {
	return &VerifierNode{
		BaseNode: NewBaseNode(id, "verifier"),
		// clock:              lamport.NewClock(),
		processingCapacity: capacity,
		threshold:          threshold,
		verificationLocks:  make(map[string]bool),
		lockRequests:       make(map[string]map[string]int),
		lockReplies:        make(map[string]map[string]bool),
		otherVerifiers:     make(map[string]bool),
		recentReadings:     make([]float32, 0, utils.VALUES_TO_STORE),
	}
}

// Start begins the verifier's operation
func (v *VerifierNode) Start() error {
	format.Display(format.Format_d("Start()", "node_verifier.go", "Starting verifier node "+v.GetName()))

	v.isRunning = true

	return nil
}

// HandleMessage processes incoming messages from control app
func (v *VerifierNode) HandleMessage(channel chan string) {

	for msg := range channel {
		var msg_clock string = format.Findval(msg, "clk", v.GetName())

		// Update Lamport clock based on received message
		msg_clock_int, _ := strconv.Atoi(msg_clock)
		v.clock = utils.Synchronise(v.clock, msg_clock_int)

		var msg_type string = format.Findval(msg, "type", v.GetName())
		var msg_content_value string = format.Findval(msg, "content_value", v.GetName())

		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store
			format.Display(format.Format_d(
				v.GetName(), "HandleMessage",
				v.GetName()+" received the new reading <"+msg_content_value+">"))

			// Parse the reading value
			readingVal, err := strconv.ParseFloat(msg_content_value, 32)
			if err != nil {
				log.Printf("%s: Error parsing reading value '%s': %v", v.GetName(), msg_content_value, err)
				continue // Skip this message if parsing fails
			}

			// Add to FIFO queue
			if len(v.recentReadings) >= utils.VALUES_TO_STORE {
				// Remove the oldest element (slice trick)
				v.recentReadings = v.recentReadings[1:]
			}
			v.recentReadings = append(v.recentReadings, float32(readingVal))

			// Optional: Log the current queue state for debugging
			// log.Printf("%s: Current readings queue: %v", v.GetName(), v.recentReadings)

		}
	}

}
