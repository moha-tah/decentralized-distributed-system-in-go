package node

import (
	"time"
	// "bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	// "os"    // Use for the bufio reader: reads from os stdin
	//  "strings"
	"strconv"
	"distributed_system/format"
	"distributed_system/utils"
)

// UserNode represents a user in the system
type UserNode struct {
    BaseNode
    // store           *storage.DataStore
    predictionModel string
    predictionWindow time.Duration
    predictionInterval time.Duration
    lastPrediction  *float32 // pointer to allow nil value at start
}

// NewUserNode creates a new user node
func NewUserNode(id string, model string, window time.Duration) *UserNode {
    return &UserNode{
        BaseNode:          NewBaseNode(id, "user"),
        // store:             storage.NewDataStore(15), // 15 days retention
        predictionModel:   model,
        predictionWindow:  window,
        predictionInterval: 3 * time.Second, // Make new predictions hourly
        lastPrediction:    nil,
    }
}


// Start begins the verifier's operation
func (u *UserNode) Start()  error {
	format.Display(format.Format_d("node_user.go", "Start()", "Starting user node " + u.GetName()))

	u.isRunning = true

	// go func() {
	//
	// 	reader := bufio.NewReader(os.Stdin)
	//
	// 	for {
	// 	    rcvmsg, err := reader.ReadString('\n') // read until newline
	// 	    if err != nil {
	// 		// Handle error properly, e.g., if the connection is closed
	// 		format.Display(format.Format_e("node_user.go", "Start()", "Reading error: " + err.Error()))
	// 		return
	// 	    }
	// 	    rcvmsg = strings.TrimSuffix(rcvmsg, "\n") // remove the trailing newline character
	// 	    // u.HandleMessage(rcvmsg)
	// 	}
	//     }()


	return nil
}


// HandleMessage processes incoming messages
func (u *UserNode) HandleMessage(channel chan string) {

	for msg := range channel {
		var msg_clock string = format.Findval(msg, "clk", u.GetName())
	    
		// Update Lamport clock based on received message
		msg_clock_int, _ := strconv.Atoi(msg_clock)
		u.clock = utils.Synchronise(u.clock,  msg_clock_int)


		var msg_type string = format.Findval(msg, "type", u.GetName())
		var msg_content_value string = format.Findval(msg, "content_value", u.GetName())
	    
		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store
			format.Display(format.Format_d(
				"node_user.go", "HandleMessage()",
				u.GetName() + " received the new reading <" + msg_content_value + ">"))
		}

	}
}
