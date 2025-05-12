package node

import (
	"time"
	// "bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	// "os"    // Use for the bufio reader: reads from os stdin
	//  "strings"
	"distributed_system/format"
	"distributed_system/utils"
	"log" // Added for logging errors
	"strconv"
)

// UserNode represents a user in the system
type UserNode struct {
	BaseNode
	// store           *storage.DataStore
	predictionModel    string
	predictionWindow   time.Duration
	predictionInterval time.Duration
	lastPrediction     *float32             // pointer to allow nil value at start
	recentReadings     map[string][]float32 // FIFO queue per sender
}

// NewUserNode creates a new user node
func NewUserNode(id string, model string, window time.Duration) *UserNode {
	return &UserNode{
		BaseNode:           NewBaseNode(id, "user"),
		predictionModel:    model,
		predictionWindow:   window,
		predictionInterval: 3 * time.Second, // Make new predictions hourly
		lastPrediction:     nil,
		recentReadings:     make(map[string][]float32),
	}
}

// Start begins the verifier's operation
func (u *UserNode) Start() error {
	format.Display(format.Format_d("node_user.go", "Start()", "Starting user node "+u.GetName()))

	u.isRunning = true

	return nil
}

// HandleMessage processes incoming messages from control layer
func (u *UserNode) HandleMessage(channel chan string) {

	for msg := range channel {
		var msg_clock string = format.Findval(msg, "clk", u.GetName())

		// Update Lamport clock based on received message
		msg_clock_int, _ := strconv.Atoi(msg_clock)
		u.clock = utils.Synchronise(u.clock, msg_clock_int)

		var msg_type string = format.Findval(msg, "type", u.GetName())
		var msg_content_value string = format.Findval(msg, "content_value", u.GetName())
		var msg_sender string = format.Findval(msg, "sender_name", u.GetName())

		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store for the specific sender
			format.Display(format.Format_d(
				"node_user.go", "HandleMessage()",
				u.GetName()+" received the new reading <"+msg_content_value+"> from "+msg_sender))

			// Parse the reading value
			readingVal, err := strconv.ParseFloat(msg_content_value, 32)
			if err != nil {
				log.Printf("%s: Error parsing reading value '%s': %v", u.GetName(), msg_content_value, err)
				continue // Skip this message if parsing fails
			}

			// Get or create the queue for the sender
			queue, exists := u.recentReadings[msg_sender]
			if !exists {
				queue = make([]float32, 0, utils.VALUES_TO_STORE)
			}

			// Add to FIFO queue for this sender
			if len(queue) >= utils.VALUES_TO_STORE {
				// Remove the oldest element (slice trick)
				queue = queue[1:]
			}
			queue = append(queue, float32(readingVal))
			u.recentReadings[msg_sender] = queue // Update the map

			// Optional: Log the current queue state for debugging
			// log.Printf("%s: Current readings queue for %s: %v", u.GetName(), msg_sender, queue)
		}

	}
}
