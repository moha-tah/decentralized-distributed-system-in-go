package node

import (
	"bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"distributed_system/format"
	"distributed_system/utils"
	"errors"
	"os" // Use for the bufio reader: reads from os stdin
	"strconv"
	"strings"
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
	seenIDs   	       map[string]struct{}
	channel_to_application chan string

	nbOfKnownSites		int
	knownSiteNames		[]string
	siteAskedDiscovery	string // Site responsible for the ongoing discovery
}
func (c *ControlLayer) GetName() string {
	return c.nodeType + " (" + c.id + ")"
}
func (c *ControlLayer) GenerateUniqueMessageID() string {
	return "control_" + c.id + "_" + strconv.Itoa(c.nbMsgSent)
}


func NewControlLayer(id string, child Node) *ControlLayer {
    return &ControlLayer{
		id: id,
		nodeType: "control",
		isRunning: false,
		clock: 	   0,
		child:	   child,
		nbMsgSent: 0,
		seenIDs:   make(map[string]struct{}),
		channel_to_application: make(chan string),
		nbOfKnownSites: 1, // This layer knows itself at startup
    }
}

// Start begins the control operations
func (c ControlLayer) Start()  error {
	format.Display(format.Format_d("Start()", "control_layer.go", "Starting control layer " + c.GetName()))

	// TODO idea to know how many nodes exists:
	// We wan not send the pear discovery message RIGHT AT startup: other nodes won't be 
	// created yet, at the network_ring.sh script creates nodes one after the other. So 
	// the idea is to wait 2 seconds (which is much, 0,5s should be enough) for all the nodes 
	// to be created, then sending the pear discovery message 
	// go func() {
	// 	time.Sleep(time.Duration(2) * time.Second)
	// 	c.SendPearDiscovery()
	// }()

	c.isRunning = true

	// Notify the child that its control layer has been created
	c.child.SetControlLayer(c)
	c.child.Start()
	go c.child.HandleMessage(c.channel_to_application) // Msg to application will be send through channel

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

	format.Display(format.Format_w(c.GetName(), "HandleMsg", "Received: " + msg))
    
	// Update Lamport clock based on received message
	var msg_clock string = format.Findval(msg, "clk", c.GetName())
	msg_clock_int, _ := strconv.Atoi(msg_clock)
	c.clock = utils.Synchronise(c.clock,  msg_clock_int)

	// Extract msg caracteristics
	var msg_destination string = format.Findval(msg, "destination", c.GetName())
	var msg_type string = format.Findval(msg, "type", c.GetName())
	var msg_content_value string = format.Findval(msg, "content_value", c.GetName())

	// Will be used at the end to check if 
	// control layer needs to resend the message to all other nodes
	var propagate_msg bool = false
    
	if msg_destination == "applications" {
		switch msg_type {
			case "new_reading":
				// Send to child app
				// c.child.HandleMessage(msg)
				c.channel_to_application <- msg // through channel

				format.Display(format.Format_d(
					c.GetName(), "HandleMessage()",
					c.GetName() + " received the new reading <" + msg_content_value + ">"))

				propagate_msg = true
		}

		if propagate_msg { // propagate to applications
			// TODO: is it appropriate to do it that way?
			// msg is comming from a distant app layer, and we need 
			// to propagate to other distant apps, so we use the below
			// function, even though is was thought as a function
			// that does localapp => send to other. It can be used as both.
			c.SendControlMsg(msg_content_value, format.Findval(msg, "content_type", c.GetName()), 
				msg_type, msg_destination, msg_id)
		}

	} else if msg_destination == "control" {
		// Control logic operations

		switch msg_type {

		case "pear_discovery":
			var sender string = format.Findval(msg, "original_sender_name", c.GetName())

			if sender == c.GetName() {
				// END OF DISCOVERY => Propagate answer to others
				format.Display(format.Format_w(c.GetName(), "HandleMsg()", "Got all answers"))

				rcv_nbOfKnownSites, _ := strconv.Atoi(format.Findval(msg, "content_value", c.GetName()))
				c.nbOfKnownSites = rcv_nbOfKnownSites
				format.Display(format.Format_e(c.GetName(), "HandleMsg()", "Updated nb sites to " + strconv.Itoa(rcv_nbOfKnownSites)))
				// c.SendControlMsg(strconv.Itoa(c.nbOfKnownSites), "nbOfKnownSites", "pear_discovery_answer",
					// "control", c.GetName(), "")

			} else {
				var nbSitesOngoing_str string = format.Findval(msg, "content_value", c.GetName())
				nbSitesOngoing_int, err := strconv.Atoi(nbSitesOngoing_str)
				if err != nil {
					format.Display(format.Format_e(c.GetName(), "HandleMessage()", "Reading error: " + err.Error()))
					return err
				}

				new_msg := format.Replaceval(msg, "content_value", strconv.Itoa(nbSitesOngoing_int + 1))

				// Propagation: 
				c.SendMsg(new_msg)

			}
		case "pear_discovery_answer":
			// Propagate answer to other, and update value of current control layer
			var nbSitesOngoing_str string = format.Findval(msg, "content_value", c.GetName())
			nbSitesOngoing_int, _ := strconv.Atoi(nbSitesOngoing_str)

			c.nbOfKnownSites = nbSitesOngoing_int // update value 
			format.Display(format.Format_e(c.GetName(), "HandleMsg()", "Updated nb sites to " + nbSitesOngoing_str))

			c.SendControlMsg(nbSitesOngoing_str, "nbOfKnownSites", "pear_discovery_answer",
				"control", "")
		}
	}

	return nil
}

// AddNewMessageId adds an entry in the seenIDs to remember
// that the control layer saw this message. 
func (c *ControlLayer) AddNewMessageId(id string) {
	c.seenIDs[id] = struct{}{} // Zero bytes of storage
}


// SendMsgFromApplication is the portal between control layer and application 
// layer: the app layer asks the control layer to send a message to the network.
// It is supposed that the application won't send two times the same message, 
// so no check if already got the message (=already seen the ID).
// => Is it a receiving action followed by a call to sending action
func (c *ControlLayer) SendApplicationMsg(msg string) error {
	var id string = format.Findval(msg, "id", c.GetName())
	if id != "" {
		c.AddNewMessageId(id)
	} else {
		return errors.New("No id in message: <" + msg + ">")
	}

	var rcv_clk string = format.Findval(msg, "clk", c.GetName())
	rcv_clk_int, _ := strconv.Atoi(rcv_clk)

	c.clock = utils.Synchronise(c.clock, rcv_clk_int) 

	c.SendMsg(msg)

	return nil
}

func (c *ControlLayer) SendControlMsg(msg_content string, msg_content_type string, 
					msg_type string,destination string, fixed_id string) error {
	var msg_id string
	if fixed_id == "" {
		msg_id = c.GenerateUniqueMessageID()
	} else {
		msg_id = fixed_id
	}

	msg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", msg_id,
			"type", msg_type,
			"sender_name", c.GetName(),
			"sender_type", "control",
			"destination", destination,
			"clk", strconv.Itoa(c.clock), // Will be replaced in c.SendMsg
			"content_type", msg_content_type,
			"content_value", msg_content))

	c.SendMsg(msg)

	return nil
}

// Sending action
func (c *ControlLayer) SendMsg(msg string) {
	c.clock = c.clock + 1
	msg = format.Replaceval(msg, "clk", strconv.Itoa(c.clock))
	format.Msg_send(msg, c.GetName())
	c.nbMsgSent = c.nbMsgSent + 1
}

// SendPearDiscovery sends a message asking for pear discovery
func (c *ControlLayer) SendPearDiscovery() {
	c.SendControlMsg(strconv.Itoa(c.nbOfKnownSites), "nbSitesPassedThrough",
		"pear_discovery", "control", "")
}
