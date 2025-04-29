package node


import (
	"strconv"
	"bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"os"    // Use for the bufio reader: reads from os stdin
	"strings"
	"errors"
	"distributed_system/format"
	"distributed_system/utils"
)

type ControlLayer struct {
	id        string
	nodeType  string
	isRunning bool
	clock     int
	child	  Node
	nbMsgSent int
	// Seen IDs of all received messages :
	// keys are IDs, empty struct{} takes zero bytes
	// To help preventing the reading of same msg if it loops
	seenIDs   map[string]struct{}
	channel_to_application chan string
}
func (c *ControlLayer) GetName() string {
	return c.nodeType + " (" + c.id + ")"
}


func NewControlLayer(id string, child Node) *ControlLayer {
    return &ControlLayer{
		id: id,
		isRunning: false,
		clock: 	   0,
		child:	   child,
		nbMsgSent: 0,
		seenIDs:   make(map[string]struct{}),
		channel_to_application: make(chan string),
    }
}

// Start begins the control operations
func (c ControlLayer) Start()  error {
	format.Display(format.Format_d("Start()", "control_layer.go", "Starting control layer " + c.GetName()))

	c.isRunning = true

	// Notify the child that its control layer has been created
	c.child.SetControlLayer(c)
	c.child.Start()
	go c.child.HandleMessage(c.channel_to_application)

	go func() {
    
		reader := bufio.NewReader(os.Stdin)

		for {
			rcvmsg, err := reader.ReadString('\n') // read until newline
		    	if err != nil {
				// Handle error properly, e.g., if the connection is closed
				format.Display(format.Format_e("Start()", "control_layer.go", "Reading error: " + err.Error()))
				return
			    }
		    	rcvmsg = strings.TrimSuffix(rcvmsg, "\n") // remove the trailing newline character
			c.HandleMessage(rcvmsg)
		}
	}()
	select{}
}


// HandleMessage processes incoming messages
func (c *ControlLayer) HandleMessage(msg string) error {

	// Make sure we never saw this message before.
	// It might happen eg. in a bidirectionnal ring 
	var msg_id string = format.Findval(msg, "id", c.GetName())
	if _, ok := c.seenIDs[msg_id]; ok {
		return nil
	}
	c.AddNewMessageId(msg_id)

	var msg_clock string = format.Findval(msg, "clk", c.GetName())
    
	// Update Lamport clock based on received message
	msg_clock_int, _ := strconv.Atoi(msg_clock)
	c.clock = utils.Synchronise(c.clock,  msg_clock_int)


	var msg_destination string = format.Findval(msg, "destination", c.GetName())
	var msg_type string = format.Findval(msg, "type", c.GetName())
	var msg_content_value string = format.Findval(msg, "content_value", c.GetName())

	var propagate_msg bool = false
    
	if msg_destination == "applications" {
		switch msg_type {
			case "new_reading":
				// Send to child app
				// c.child.HandleMessage(msg)
				c.channel_to_application <- msg

				format.Display(format.Format_d(
					c.GetName(), "HandleMessage()",
					c.GetName() + " received the new reading <" + msg_content_value + ">"))

				propagate_msg = true
		}
	} else if msg_destination == "control" {
		// Control logic operations
	}

	if propagate_msg {
		format.Msg_send(msg, c.GetName())
		c.nbMsgSent = c.nbMsgSent + 1
	}

	return nil
}

func (c *ControlLayer) AddNewMessageId(id string) {
	c.seenIDs[id] = struct{}{}
}

func (c *ControlLayer) SendMsgFromApplication(msg string) error {

	// time.Sleep(time.Duration(1) * time.Second)

	var id string = format.Findval(msg, "id", c.GetName())
	if id != "" {
		c.AddNewMessageId(id)
	} else {
		return errors.New("No id in message: <" + msg + ">")
	}
	format.Msg_send(msg, c.GetName())
	return nil
}
