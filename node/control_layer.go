package node

import (
	"bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"distributed_system/format"
	"distributed_system/utils"
	"errors"
	"os" // Use for the bufio reader: reads from os stdin
	"strconv"
	"strings"
	"time"
)

type ControlLayer struct {
	// mu   	  sync.RWMutex
	id        string
	nodeType  string
	isRunning bool
	clock     int
	child     Node
	nbMsgSent uint64
	// Seen IDs of all received messages :
	// keys are IDs, empty struct{} takes zero bytes
	// To help preventing the reading of same msg if it loops
	seenIDs                map[string]struct{}
	channel_to_application chan string

	nbOfKnownSites       int
	knownSiteNames       []string
	sentDiscoveryMessage bool // To send its name only once
	pearDiscoverySealed  bool
}

func (c *ControlLayer) GetName() string {
	return c.nodeType + " (" + c.id + ")"
}
func (c *ControlLayer) GenerateUniqueMessageID() string {
	return "control_" + c.id + "_" + strconv.Itoa(int(c.nbMsgSent))
}

func NewControlLayer(id string, child Node) *ControlLayer {
	return &ControlLayer{
		id:                     id,
		nodeType:               "control",
		isRunning:              false,
		clock:                  0,
		child:                  child,
		nbMsgSent:              0,
		seenIDs:                make(map[string]struct{}),
		channel_to_application: make(chan string),
		nbOfKnownSites:         1, // This layer knows itself at startup
		sentDiscoveryMessage:   false,
		pearDiscoverySealed:    false,
	}
}

// Start begins the control operations
func (c *ControlLayer) Start() error {
	format.Display(format.Format_d("Start()", "control_layer.go", "Starting control layer "+c.GetName()))

	// Notify child that this is its control layer it must talk to.
	c.child.SetControlLayer(*c)

	// TODO idea to know how many nodes exists:
	// We can not send the pear discovery message RIGHT AT startup: other nodes won't be
	// created yet, at the network_ring.sh script creates nodes one after the other. So
	// the idea is to wait 1 seconds (which is much, 0,1s should be enough) for all the nodes
	// to be created, then sending the pear discovery message.
	// We then suppose that all the response will be aquired by the node 0 (only node which makes
	// the call) after 1 seconds.
	// 🔥ONLY node whose id is zero will send this message.
	if c.id == "0_control" {
		go func() {
			time.Sleep(time.Duration(1) * time.Second)
			c.SendPearDiscovery()
			time.Sleep(time.Duration(1) * time.Second)
			c.ClosePearDiscovery() // Will send known names to all nodes

			// Start the child only after
			c.child.Start()
			go c.child.HandleMessage(c.channel_to_application) // Msg to application will be send through channel
		}()
	} else {
		go func() { // Wake up child only after pear discovery is finished
			time.Sleep(time.Duration(2) * time.Second)

			// Notify the child that its control layer has been created
			c.child.Start()
			go c.child.HandleMessage(c.channel_to_application) // Msg to application will be send through channel
		}()
	}

	c.isRunning = true

	go func() {

		reader := bufio.NewReader(os.Stdin)

		for {
			rcvmsg, err := reader.ReadString('\n') // read until newline
			if err != nil {
				// Handle error properly, e.g., if the connection is closed
				format.Display(format.Format_e("Start()", "control_layer.go", "Reading error: "+err.Error()))
				return
			}
			rcvmsg = strings.TrimSuffix(rcvmsg, "\n") // remove the trailing newline character
			c.HandleMessage(rcvmsg)
		}
	}()
	select {}
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

	if !strings.Contains(msg, "pear_discovery") {
		// Only print to stderr msg that are not related to pear_discovery
		// (overwhelming)
		format.Display(format.Format_w(c.GetName(), "HandleMsg()", "Received: "+msg))
	}

	// Update Lamport clock based on received message
	var msg_clock string = format.Findval(msg, "clk", c.GetName())
	msg_clock_int, _ := strconv.Atoi(msg_clock)
	c.clock = utils.Synchronise(c.clock, msg_clock_int)

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
				c.GetName()+" received the new reading <"+msg_content_value+">"))

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
			if !c.sentDiscoveryMessage && c.id != "0_control" {
				// Send response (current node's name) to the control layer responsible
				// of pear discovery. This message will be sent only once thanks
				// to c.sentDiscoveryMessage. The initiator of pear discovery won't send
				// its name to itself.
				// 🔥 Modify pear_discovery_answer if the bool is not used anymore.
				c.SendControlMsg(c.GetName(), "siteName", "pear_discovery_answer",
					format.Findval(msg, "content_value", c.GetName()), "")

				// Then propagate the pear_discovery
				var propagate_msg string = format.Replaceval(msg, "sender_name", c.GetName())
				c.SendMsg(propagate_msg)

				c.sentDiscoveryMessage = true

			}
		case "pear_discovery_sealing":

			if !c.pearDiscoverySealed {
				c.pearDiscoverySealed = true
				// 💡The node responsible of pear discovery will also do the following, as
				// no condition on node id. Done that way in order to have the message dispayling
				// the number of known nodes, even for the node 0.
				var names_in_message string = format.Findval(msg, "content_value", c.GetName())

				c.knownSiteNames = strings.SplitN(names_in_message, "@", -1)
				c.nbOfKnownSites = len(c.knownSiteNames)

				format.Display(format.Format_e(c.GetName(), "HandleMsg()", "Updated nb sites to "+strconv.Itoa(c.nbOfKnownSites)))

				// Propagate answer to other
				var propagate_msg string = format.Replaceval(msg, "sender_name", c.GetName())
				// New id to make sure initiator will receive it:
				propagate_msg = format.Replaceval(msg, "id", c.GenerateUniqueMessageID())
				c.SendMsg(propagate_msg)
			}

		}
	} else if msg_destination == c.GetName() { // The msg is only for the current node
		switch msg_type {
		case "pear_discovery_answer":
			// Only the node responsible for the pear discovery will read these
			// lines, as direct messaging only target this node in pear discovery process.
			var newSiteName string = format.Findval(msg, "content_value", c.GetName())

			// Check if not already in known sites list. Will happend because of answer
			// message propagation (see case below) (which is a mandatory feature
			// -to bring message all the way to node 0- not a problem)
			if !strings.Contains(
				strings.Join(c.knownSiteNames, "@"), // main string in which to check
				newSiteName,                         // site name checked
			) {
				c.knownSiteNames = append(c.knownSiteNames, newSiteName)
			}
		}
	} else {
		switch msg_type {
		case "pear_discovery_answer":
			// Here, a node receives the answer (name) of another node.
			// So it must propagate this answer so that the node 0
			// receives the answer. When propagating, it must keep
			// the same content_value, but can modify the sender_name (as
			// the node 0 fetches names in the content_value message field).

			// Change name to keep the messaging logic (sender name = name of the
			// node which send the current message).
			var propagation_message string = format.Replaceval(msg, "sender_name", c.GetName())
			c.SendMsg(propagation_message)

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
	msg_type string, destination string, fixed_id string) error {
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
			"clk", strconv.Itoa(int(c.clock)), // Will be replaced in c.SendMsg
			"content_type", msg_content_type,
			"content_value", msg_content))

	c.SendMsg(msg)

	return nil
}

// Sending action
func (c *ControlLayer) SendMsg(msg string) {
	// Snapshot the clock under a read‐lock:
	c.clock = c.clock + 1

	msg = format.Replaceval(msg, "clk", strconv.Itoa(c.clock))
	format.Msg_send(msg, c.GetName())

	c.nbMsgSent = c.nbMsgSent + 1

}

// SendPearDiscovery sends a message asking for pear discovery
// The content_value of the message is the name to which the
// nodes must answer (in this project, is fixed to node id 0)
// (TODO: modify above if changed)
// 🔥ONLY node whose id is zero will send the pear discovery message.
func (c *ControlLayer) SendPearDiscovery() {
	c.SendControlMsg(c.GetName(), "siteName", "pear_discovery", "control", "")
}

// When closing, node 0 will send the names it acquired during
// pear discovery, so that all nodes can know which nodes are
// in the network.
// 🔥ONLY node whose id is zero will send this message.
func (c *ControlLayer) ClosePearDiscovery() {
	// Append the current nodes name (node responsible of the pear dis.)
	// as it did not received its own name.
	c.knownSiteNames = append(c.knownSiteNames, c.GetName())

	c.SendControlMsg(strings.Join(c.knownSiteNames, "@"),
		"siteNames", "pear_discovery_sealing", "control", "")
}
