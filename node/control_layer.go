package node

import (
	"bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"distributed_system/format"
	"distributed_system/utils"
	// "encoding/csv"
	// "fmt"
	// "log"
	"os" // Use for the bufio reader: reads from os stdin
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ControlLayer struct {
	mu   	  sync.RWMutex
	id        string
	nodeType  string
	isRunning bool

	clk 		int
	vectorClockReady bool  // true when nbOfKnownSites is set
	vectorClock      []int // Size = nbOfKnownSites when init.
	nodeIndex        int   // position of this node in the vector

	child     Node
	nbMsgSent uint64
	// Seen IDs of all received messages:
	// To help preventing the reading of same msg if it loops
	IDWatcher              *utils.MIDWatcher
	channel_to_application chan string

	nbOfKnownSites       int
	knownSiteNames       []string
	knownVerifierNames   []string 
	sentDiscoveryMessage bool // To send its name only once
	pearDiscoverySealed  bool
	receivedSnapshots    map[string]SnapshotData

	// For the snapshot algorithm:
	markersReceivedFrom 	map[int][]string 	// map[snapshot_id] => list of nodes that sent back the marker for that snap_id
	nodeMarker 		int 			// current node's marker id 
	subTreeState		GlobalSnapshot		// Buffered, waiting for acquiring all to the send to parent
	snapResponseSent	bool			// true if the snapshot response has been sent to the parent
	nbSnapResponsePending	int			// number of snapshot responses pending (= 0 => send our response to parent)

	// For the spanning tree:
	parentNodeName string // name of the parent node in the spanning tree
	childrenNames []string // names of the children nodes in the spanning tree
	directNeighbors	     []string
	directNeighborsEnd	bool // true when we know our neighbors (delay of 1s after neighbor discovery)
	nbExpectedTreeAnswers     int
}


func (c *ControlLayer) GetName() string {
	return c.nodeType + " (" + c.id + ")"
}
func (c *ControlLayer) GenerateUniqueMessageID() string {
	return "control_" + c.id + "_" + strconv.Itoa(int(c.nbMsgSent))
}

func NewControlLayer(id string, child Node) *ControlLayer {
    return &ControlLayer{
		id: id,
		nodeType: "control",
		isRunning: false,
		vectorClock: []int{0},
		clk: 0,
		nodeIndex:  0,
		child:	   child,
		nbMsgSent: 0,
		IDWatcher:  utils.NewMIDWatcher(),
		channel_to_application: make(chan string, 10), // Buffered channel
		nbOfKnownSites: 0, 
		sentDiscoveryMessage: false,
		pearDiscoverySealed: false,
		knownSiteNames: make([]string, 0),
		knownVerifierNames: make([]string, 0),
		receivedSnapshots:      make(map[string]SnapshotData),

		// For the snapshot algorithm:
		markersReceivedFrom: make(map[int][]string),
		nodeMarker: -1,
		subTreeState: GlobalSnapshot{SnapshotId: ""},

		// For the spanning tree:
		directNeighbors:     	make([]string, 0),
		directNeighborsEnd:	false,
		parentNodeName: 	"",
		childrenNames: 		make([]string, 0),
		nbExpectedTreeAnswers: 	0,
    }
}

// Start begins the control operations
func (c *ControlLayer) Start() error {
	format.Display(format.Format_d(c.GetName(), "Start()", "Starting control layer "+c.GetName()))

	// Notify child that this is its control layer it must talk to.
	c.child.SetControlLayer(c)

	// Msg to application will be send through channel
	go c.child.HandleMessage(c.channel_to_application) 

	// Idea to know how many nodes exists:
	// We can not send the pear discovery message RIGHT AT startup: other nodes won't be
	// created yet, at the network_ring.sh script creates nodes one after the other. So
	// the idea is to wait 1 seconds (which is much, 0,1s should be enough) for all the nodes
	// to be created, then sending the pear discovery message.
	// We then suppose that all the response will be aquired by the node 0 (only node which makes
	// the call) after 1 seconds.
	// 🔥ONLY node whose id is zero will send this message.
	if c.id == "0_control" {
		go func() {
			// 1. Wait for all control layer to be instanciated
			time.Sleep(time.Duration(1) * time.Second)

			// 2. Send pear discovery to know all nodes in the network
			c.SendPearDiscovery()
			time.Sleep(time.Duration(2) * time.Second)

			// 3. Close the pear discovery (=send all received names to all nodes)
			c.ClosePearDiscovery() // Will send known names to all nodes
			time.Sleep(time.Duration(1) * time.Second)
			
			// 4. Start tree construction only after we know our neighbors
			c.mu.Lock()
			directNeighborsEnd := c.directNeighborsEnd
			c.mu.Unlock()
			for directNeighborsEnd == false {
				time.Sleep(time.Duration(100) * time.Millisecond)
				c.mu.Lock()
				directNeighborsEnd = c.directNeighborsEnd
				c.mu.Unlock()
			}
			c.SendTreeConstruction()
		}()
		//demande de snapshot après 5 secondes
		go func() {
			time.Sleep(6 * time.Second) // attendre que tout soit initialisé
			format.Display(format.Format_d("Start()", c.GetName(), "⏳ 20s écoulées, envoi de la requête snapshot..."))
			c.RequestSnapshot()
			format.Display(format.Format_d("Start()", c.GetName(), "📤 snapshot_request envoyé depuis "+c.GetName()))
			time.Sleep(3 * time.Second) // attendre que tout soit initialisé
			format.Display(format.Format_d("Start()", c.GetName(), "⏳ 20s écoulées, envoi de la requête snapshot..."))
			c.RequestSnapshot()
			format.Display(format.Format_d("Start()", c.GetName(), "📤 snapshot_request envoyé depuis "+c.GetName()))
		}()
	}

	// ALL nodes wait, send neighbor discovery message to their direct neighbors, 
	// and then start their child.
	go func() { // Wake up child only after pear discovery is finished
		time.Sleep(time.Duration(4) * time.Second)
		c.SendNeighborDiscovery()
		time.Sleep(time.Duration(1) * time.Second)
		c.mu.Lock()
		c.directNeighborsEnd = true // We know our directNeighbors
		c.mu.Unlock()

		c.child.Start()
	}()

	c.isRunning = true

	// Go routine to read messages from stdin and call HandleMessage() on each new message.
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			rcvmsg, err := reader.ReadString('\n') // read until newline
			if err != nil {
				// Handle error properly, e.g., if the connection is closed
				format.Display(format.Format_e(c.GetName(), "Start()", "Reading error: "+err.Error()))
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
	// It might happen eg. in a bidirectionnal ring.
	// If it is the case (= duplicate) => ignore.
	if c.SawThatMessageBefore(msg) {
		return nil
	} 

	// Receiving operations: update the vector clock and the logical clock
	c.mu.Lock()
	if c.vectorClockReady {
		// Update the vector clock 
		recVC := format.RetrieveVectorClock(msg, len(c.vectorClock))
		c.vectorClock = utils.SynchroniseVectorClock(c.vectorClock, recVC, c.nodeIndex)
	}
	resClk, _ := strconv.Atoi(format.Findval(msg, "clk", c.GetName()))
	c.clk = utils.Synchronise(c.clk, resClk)
	c.mu.Unlock()

	
	// BEFORE any processing, snapshots are considered, as snapshot logic is in 
	// another function. 
	var processMessage bool = c.handleSnapshotMsg(msg) // process=True if normal message
	if !processMessage {
		return nil
	}

	// Extract msg caracteristics
	var msg_destination string = format.Findval(msg, "destination", c.GetName())
	var msg_type string = format.Findval(msg, "type", c.GetName())
	var sender_name_source string = format.Findval(msg, "sender_name_source", c.GetName())

	// Will be used at the end to check if
	// control layer needs to resend the message to all other nodes
	var propagate_msg bool = false

	if msg_destination == "applications" {
		switch msg_type {
		case "new_reading":
			c.SendMsg(msg, true) // Send to child app
			propagate_msg = true
		}
	} else if msg_destination == "control" { // Control logic operations
		switch msg_type {

		case "neighbor_discovery":

			msg_to_neighbor := format.Msg_format_multi(format.Build_msg_args(
				"id", c.GenerateUniqueMessageID(),
				"type", "neighbor_discovery_answer",
				"sender_name_source", c.GetName(),
				"sender_name", c.GetName(),
				"sender_type", "control",
				"destination", sender_name_source,
				"clk", "", // changed in SendMsg
				"vector_clock", "", // changed in SendMsg
				"propagation", "false",
				))
			c.SendMsg(msg_to_neighbor)
			// no propagation of the message, as it is only for the sender
		// The case "neighbor_discovery_answer" is in condition `msg_destination == c.GetName()`

		case "pear_discovery":
			if !c.sentDiscoveryMessage && c.id != "0_control" {
				// Send response (current node's name) to the control layer responsible
				// of pear discovery. This message will be sent only once thanks
				// to c.sentDiscoveryMessage. The initiator of pear discovery won't send
				// its name to itself.
				// 🔥 Modify pear_discovery_answer if the bool is not used anymore.

				answer := c.GetName()
				// If child is verifier, need to send the name to the PearD responsible,
				// as all verifiers must know the name of the other verifiers.
				if c.child.Type() == "verifier" {
					answer += utils.PearD_VERIFIER_SEPARATOR + c.child.GetName()
				}

				c.SendControlMsg(
					// Content of the message:
					answer,
					// Content type and msg type:
					"siteName", "pear_discovery_answer", 
					// Destination = source written in content_value:
					format.Findval(msg, "content_value", c.GetName()), 
					// Msg ID and source: 
					"", c.GetName())

				// Then propagate the pear_discovery
				var propagated_msg string = format.Replaceval(msg, "sender_name", c.GetName())
				c.SendMsg(propagated_msg)
				c.sentDiscoveryMessage = true // Do not send it another time
			}
		case "pear_discovery_sealing":

			if !c.pearDiscoverySealed {
				var names_in_message string = format.Findval(msg, "content_value", c.GetName())

				// for each site, retrieve its name and verifier name (if applicable):
				sites_verifiers_parts := strings.Split(names_in_message, utils.PearD_VERIFIER_SEPARATOR)
				sites_parts := strings.Split(sites_verifiers_parts[0], utils.PearD_SITE_SEPARATOR)
				
				c.mu.Lock()
				for _, site := range sites_parts {
					c.knownSiteNames = append(c.knownSiteNames, site)
				}
				if len(sites_verifiers_parts) >= 1 {
					verifiers_parts := sites_verifiers_parts[1]
					verifiers := strings.Split(verifiers_parts, utils.PearD_SITE_SEPARATOR)
					c.knownVerifierNames = append(c.knownVerifierNames, verifiers...)
				}
				c.nbOfKnownSites = len(c.knownSiteNames)
				knownSiteNames := c.knownSiteNames
				c.pearDiscoverySealed = true
				c.mu.Unlock()


				// Propagation à d'autres contrôleurs
				var propagate_msg string = format.Replaceval(msg, "sender_name", c.GetName())
				c.SendMsg(propagate_msg)


				// Init child vector clock with the known sites
				c.InitVectorClockWithSites(knownSiteNames)
				c.child.InitVectorClockWithSites(knownSiteNames)

				// If any verifier name, send it to verifier chile (if it is a verifier)
				if c.child.Type() == "verifier" {
					// Send to child app through channel 
					msg_to_verifier := format.Msg_format_multi(format.Build_msg_args(
						"id", format.Findval(msg, "id", c.GetName()),
						"type", "pear_discovery_verifier",
						"sender_name_source", c.GetName(),
						"sender_name", c.GetName(),
						"sender_type", "control",
						"destination", c.child.GetName(),
						"content_type", "siteNames",
						"content_value", strings.Join(c.knownVerifierNames, utils.PearD_SITE_SEPARATOR),
						"clk", "", // changed in SendMsg
						"vector_clock", "", // changed in SendMsg
						))
					c.SendMsg(msg_to_verifier, true) // through channel
				}

				format.Display(format.Format_g(c.GetName(), "HandleMsg()", "Updated nb sites to "+strconv.Itoa(c.nbOfKnownSites)))
			}

		case "tree_blue":
			c.processBlueTree(msg)
			
		}
	} else if msg_destination == c.GetName() { // The msg is only for the current node
		switch msg_type {
		case "pear_discovery_answer":
			// Only the node responsible for the pear discovery will read these
			// lines, as direct messaging only target this node in pear discovery process.
			var newSiteName string = format.Findval(msg, "content_value", c.GetName())

			// Extract the verifier name (if applicable):
			var verifierName string = ""
			var siteName string = ""
			if strings.Contains(newSiteName, utils.PearD_VERIFIER_SEPARATOR) {
				// Split the verifier name from the site name 
				parts := strings.Split(newSiteName, utils.PearD_VERIFIER_SEPARATOR)
				verifierName = parts[1]
				siteName = parts[0]
			} else {
				// No verifier name, only the site name 
				siteName = newSiteName
			}

			// Check if not already in known sites list. Will happend because of answer
			// message propagation (see case `pear_discovery_answer` below) 
			//(which is a mandatory feature -to bring message all the way to node 0- not a problem)
			if !slices.Contains(c.knownSiteNames, siteName) {
				c.knownSiteNames = append(c.knownSiteNames, siteName)
				c.nbOfKnownSites += 1
			} 
			if verifierName != "" && !slices.Contains(c.knownVerifierNames, verifierName) {
				c.knownVerifierNames = append(c.knownVerifierNames, verifierName)
			}
		case "neighbor_discovery_answer":
			already_discovered := slices.Contains(c.directNeighbors, sender_name_source)
			if !already_discovered {
				c.directNeighbors = append(c.directNeighbors, sender_name_source)
				format.Display(format.Format_d(c.GetName(), "HandleMessage()", "Neighbor discovered: "+sender_name_source))
				c.nbExpectedTreeAnswers += 1
			}

		case "tree_blue": // tree construction message (blue messages from lecture)
			c.processBlueTree(msg)
		case "tree_red":  // tree construction message (red messages from lecture)
			is_my_child_str := format.Findval(msg, "child", c.GetName())
			is_my_child, err := strconv.ParseBool(is_my_child_str)
			if is_my_child_str == "" {
				is_my_child = false 
			} else if err != nil {
				format.Display(format.Format_e(c.GetName(), "HandleMessage()", "Error parsing child value: "+err.Error()))
			}

			c.mu.Lock()
			c.nbExpectedTreeAnswers -= 1 
			nbExpectedTreeAnswers := c.nbExpectedTreeAnswers
			parentNodeName := c.parentNodeName
			if is_my_child {
				c.childrenNames = append(c.childrenNames, sender_name_source)
			} 
			c.mu.Unlock()
			if nbExpectedTreeAnswers == 0 {
				if parentNodeName == c.GetName() {
					format.Display(format.Format_g(c.GetName(), "HandleMessage()", "✅󰐅  All tree answers received"))
				} else {
					tree_answer := format.Msg_format_multi(format.Build_msg_args(
						"id", c.GenerateUniqueMessageID(),
						"type", "tree_red",
						"sender_name_source", c.GetName(),
						"sender_name", c.GetName(),
						"sender_type", "control",
						"destination", parentNodeName,
						"clk", "", 		// changed in SendMsg
						"vector_clock", "", 	// changed in SendMsg
						"child", "true",	// Notify parent that I am a child
					))
					c.SendMsg(tree_answer)
				}
			}

		}

	} else if msg_destination == "verifiers" {
		if c.child.Type() == "verifier" {
			// The msg is for the child app layer (which is a verifier)
			c.SendMsg(msg, true) // Send to child through channel
		} else if c.child.Type() == "user" && msg_type == "lock_release_and_verified_value" {
			// The msg is for the child app layer (which is a user)
			// The user also needs to receive the verified value:
			c.SendMsg(msg, true) // through channel 
		}
		propagate_msg = true // Propagate to other verifiers
	} else if msg_destination == c.child.GetName() {
		// The msg is directly for the child app layer
		c.SendMsg(msg, true) // through channel
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

		case "lock_release_and_verified_value":
			// This type of message if from verifier, to verifier and also users
			// as is contains the verified value. For verifier, it would have
			// entered above (case dest=verifier). And for user it is done here:
			if c.child.Type() == "user" {
				// Send to child app through channel 
				c.SendMsg(msg, true) // through channel
				// Propagate to other verifiers
				propagate_msg = true
			}
		default:
			propagate_msg = true
		}
	}

	// Propagate the message to other nodes if needed
	msg_propagate_field := format.Findval(msg, "propagation", c.GetName())
	if propagate_msg && msg_propagate_field != "false" { // Check if propagation disabled for this msg
		c.propagateMessage(msg)
	}

	return nil
}



// handleSnapshotMsg handles snapshot messages and takes appropriate actions.
// It checks if the message is a snapshot request, marker, or response and processes it accordingly.
// It also checks if the message is a normal message and updates the buffered messages if needed.
// It returns true if the message should be processed further in HandleMessage() (not a snapshot message).
// Overall logic: 
// - If a snapshot request is received, take a snapshot and propagate it.
// - If a snapshot marker is received, take a snapshot if not already taken and propagate it.
// - If a snapshot response is received, update the local state and send a response to the parent node.
// - If a normal message is received, check if it is buffered and update the buffered messages accordingly.
// ⚠️ FIFO hypothesis.
func (c *ControlLayer) handleSnapshotMsg(msg string) bool {
	msg_type := format.Findval(msg, "type", c.GetName()) 
	initiator := format.Findval(msg, "sender_name_source", c.GetName())
	sender_name := format.Findval(msg, "sender_name", c.GetName())

	snapshot_id,snap_err := strconv.Atoi(format.Findval(msg, "snapshot_id", c.GetName()))
	if snap_err != nil {
		format.Display(format.Format_e(c.GetName(), "HandleMessage()", "Error parsing snapshot_id: "+snap_err.Error()))
	}

	// Useful variable for later: Do we already have a snapshot for this id?
	c.mu.Lock()
	snapshot_taken := c.subTreeState.SnapshotId == strconv.Itoa(snapshot_id)
	if c.subTreeState.SnapshotId == "" {
		snapshot_taken = false
	}
	c.mu.Unlock()

	// Used in HandleMessage() to check if message needs to be processed (true if it is not a snapshot message)
	processMessage := false 

	//⚠️ FIFO hypothsesis: if a node receives a snapshot request, it has already
	// received a marker before from the same node.
	if msg_type == "snapshot_request" {
		if !snapshot_taken {
			c.takeSnapshotAndPropagate(snapshot_id, initiator)
		}
	} else if msg_type == "snapshot_marker" {
		if !snapshot_taken {
			c.takeSnapshotAndPropagate(snapshot_id, initiator)
			c.mu.Lock()
			c.markersReceivedFrom[snapshot_id] = append(c.markersReceivedFrom[snapshot_id], sender_name)
			c.mu.Unlock()
		} else {
			c.mu.Lock()
			already_marked := false
			for marked_snap_id := range c.markersReceivedFrom {
				if snapshot_id == marked_snap_id {
					already_marked = true
					break
				}
			}
			if !already_marked {
				c.markersReceivedFrom[snapshot_id] = append(c.markersReceivedFrom[snapshot_id], sender_name)
			}
			c.mu.Unlock()
		}

		c.mu.Lock()
		markersReceivedFrom_snapid := c.markersReceivedFrom[snapshot_id]
		directNeighbors := c.directNeighbors
		childrenNames := c.childrenNames
		c.mu.Unlock()
		if len(markersReceivedFrom_snapid) == len(directNeighbors) || len(childrenNames) == 0 {
			if !c.snapResponseSent {
				c.sendSnapshotResponse()
			}
		}
	} else if msg_type == "snapshot_response" {
		state_str := format.Findval(msg, "content_value", c.GetName())
		state := DeserializeGlobalSnapshot(state_str)
		c.mu.Lock() // Update the SubTreeState with the received state
		subTreeState := c.subTreeState
		subTreeState.VectorClock = utils.SynchroniseVectorClock(c.vectorClock, state.VectorClock, c.nodeIndex)
		for _, snapshotData := range state.Data {
			subTreeState.Data = append(subTreeState.Data, snapshotData)
		}
		c.nbSnapResponsePending -= 1
		c.subTreeState = subTreeState
		nbSnapResponses := c.nbSnapResponsePending
		c.mu.Unlock()
		// Send to parent when all children have sent their response
		if nbSnapResponses == 0  {
			c.sendSnapshotResponse()
		}
	} else {
		// Normal message, buffer it if snapshot is taken
		if snapshot_taken {
			markedThisNode := false 
			for marked_snap_id := range c.markersReceivedFrom {
				if snapshot_id == marked_snap_id {
					markedThisNode = true
					break
				}
			}
			if !markedThisNode {
				c.mu.Lock()
				// First data is current node's state.
				// It does exist as we took the snapshot (snapshot_taken == true)
				state := c.subTreeState.Data[0]
				state.BufferedMsg = append(state.BufferedMsg, msg)
				c.subTreeState.Data[0] = state
				c.mu.Unlock()
			}
		}
		processMessage = true
	}
	return processMessage
}

// sendSnapshotResponse sends the snapshot response to the parent node
func (c *ControlLayer) sendSnapshotResponse() {
	// If current node is the root => snapshot algorithm is finished
	if c.parentNodeName == c.GetName() {
		c.mu.Lock()
		format.Display(format.Format_g(c.GetName(), "SendSnap()", "✅✅✅ Snapshot algorithm finished for snapshot id "+c.subTreeState.SnapshotId))
		for _, childState := range c.subTreeState.Data {
			format.Display(format.Format_g(c.GetName(), "SendSnap()", "Child state: "+SerializeSnapshotData(childState)))
		}
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	snapshot_id := c.nodeMarker
	snap_content := SerializeGlobalSnapshot(c.subTreeState)
	parentNodeName := c.parentNodeName
	c.snapResponseSent = true
	c.mu.Unlock()
	response := format.Msg_format_multi(format.Build_msg_args(
		"id", c.GenerateUniqueMessageID(),
		"type", "snapshot_response",
		"sender_name", c.GetName(),
		"sender_name_source", c.GetName(),
		"sender_type", "control",
		"destination", parentNodeName,
		"content_type", "snapshot_data",
		"content_value", snap_content,
		"snapshot_id", strconv.Itoa(snapshot_id),
		"clk", "",		// changed in SendMsg
		"vector_clock", "",	// changed in SendMsg
	))
	c.SendMsg(response)
}


// Tree construction logic (blue messages from lecture)
func (c *ControlLayer) processBlueTree(msg string) {
	sender_name_source := format.Findval(msg, "sender_name_source", c.GetName())
	tree_answer := format.Msg_format_multi(format.Build_msg_args(
		"id", c.GenerateUniqueMessageID(),
		"type", "tree_red",
		"sender_name_source", c.GetName(),
		"sender_name", c.GetName(),
		"sender_type", "control",
		"destination", sender_name_source,
		"clk", "", 		// changed in SendMsg
		"vector_clock", "", 	// changed in SendMsg
		"child","false",
		"propagation", "false",
	))

	c.mu.Lock()
	parentNodeName := c.parentNodeName
	c.mu.Unlock()

	if parentNodeName == "" {
		format.Display(format.Format_w(c.GetName(), "HandleMessage()", "󰹼  󰚏  new parent node: "+sender_name_source))
		c.mu.Lock()
		c.parentNodeName = sender_name_source
		c.nbExpectedTreeAnswers = c.nbExpectedTreeAnswers - 1
		nbExpectedTreeAnswers := c.nbExpectedTreeAnswers
		directNeighbors := c.directNeighbors
		c.mu.Unlock()

		if nbExpectedTreeAnswers > 0 {
			for _, neighbor := range directNeighbors {
				if neighbor != sender_name_source {
					blue_msg := format.Replaceval(msg, "id", c.GenerateUniqueMessageID())
					blue_msg = format.Replaceval(blue_msg, "destination", neighbor)
					blue_msg = format.Replaceval(blue_msg, "sender_name_source", c.GetName())
					blue_msg = format.Replaceval(blue_msg, "sender_name", c.GetName())
					c.SendMsg(blue_msg)

				}
			}
		} else {
			c.SendMsg(format.Replaceval(tree_answer, "child", "true"))
		}
	} else {
		c.SendMsg(tree_answer)
	}
}

func (c* ControlLayer) takeSnapshotAndPropagate(snapshot_id int, initiator string) {
	c.mu.Lock()
	c.snapResponseSent = false
	c.nbSnapResponsePending = len(c.childrenNames)
	state := SnapshotData{
		VectorClock: c.vectorClock,
		Content:     c.child.GetLocalState(),
		Initiator:   initiator,
		NodeName: c.GetName(),
		BufferedMsg: make([]string, 0),
	}

	subTreeState := GlobalSnapshot{} // Reset current snapshot
	if subTreeState.Initiator == "" {
		subTreeState.Initiator = state.Initiator 
	}
	if subTreeState.Data == nil {
		subTreeState.Data = []SnapshotData{}
	}
	subTreeState.VectorClock = c.vectorClock
	subTreeState.Data = append(subTreeState.Data, state)
	subTreeState.SnapshotId = strconv.Itoa(snapshot_id)
	c.subTreeState = subTreeState

	c.markersReceivedFrom[c.nodeMarker] = make([]string, 0)
	c.markersReceivedFrom[c.nodeMarker] = append(c.markersReceivedFrom[c.nodeMarker], c.GetName())
	c.nodeMarker = snapshot_id

	directNeighbors := c.directNeighbors
	childrenNames := c.childrenNames
	c.mu.Unlock()

	propagate_marker := format.Msg_format_multi(format.Build_msg_args(
		"id", c.GenerateUniqueMessageID(),
		"type", "snapshot_marker",
		"sender_name", c.GetName(),
		"sender_name_source", initiator,
		"sender_type", "control",
		"destination", "",
		"clk", "", 		// Is done in c.SendMsg
		"vector_clock", "", 	// Is done in c.SendMsg
		"content_type", "request_type",
		"propagation", "false",
	))
	for _, neighbor := range directNeighbors {
		c.SendMsg(format.Replaceval(propagate_marker, "destination", neighbor))
	}

	propagate_snapshot_rq := format.Replaceval(propagate_marker, "type", "snapshot_request")
	for _, child := range childrenNames {
		c.SendMsg(format.Replaceval(propagate_snapshot_rq, "destination", child))
	}
}

// RequestSnapshot sends a snapshot request to all neighbors.
// It first takes a snapshot and then sends the request to other nodes.
// As it first takes a snapshot, nodes will first receive the 
// marker message from `takeSnapshotAndPropagate()` and then the request message.
func (c *ControlLayer) RequestSnapshot() {
	c.mu.Lock()
	snapshot_id := c.nodeMarker + 1
	c.mu.Unlock()

	c.takeSnapshotAndPropagate(snapshot_id, c.GetName())

	msgID := c.GenerateUniqueMessageID()
	msg := format.Msg_format_multi(format.Build_msg_args(
		"id", msgID,
		"type", "snapshot_request",
		"sender_name", c.GetName(),
		"sender_name_source", c.GetName(),
		"sender_type", "control",
		"destination", "",
		"clk", "", // Is done in c.SendMsg
		"vector_clock", "", // Is done in c.SendMsg
		"content_type", "request_type",
		"content_value", "snapshot_start",
		"snapshot_id", strconv.Itoa(snapshot_id),
	))
	for _, neighbor := range c.directNeighbors {
		c.SendMsg(format.Replaceval(msg, "destination", neighbor))
	}

	if !c.vectorClockReady {
		format.Display(format.Format_e(c.GetName(), "RequestSnapshot()", "Vector clock not ready"))
		panic("Vector clock not ready")
	}
}

// Propagate a message to all other nodes
func (c *ControlLayer) propagateMessage(msg string) {
	propagate_msg := format.Replaceval(msg, "sender_name", c.GetName())
	propagate_msg = format.Replaceval(msg, "sender_type", "control")
	c.SendMsg(propagate_msg)
}

// AddNewMessageId adds an entry in the seenIDs to remember
// that the control layer saw this message.
func (c *ControlLayer) AddNewMessageId(sender_name string, MID_str string) {
	msg_NbMessageSent, err := utils.MIDFromString(MID_str)
	if err != nil {
		format.Display(format.Format_e("AddNewMessageID()", c.GetName(), "Error in message id: "+err.Error()))
	}
	
	c.mu.Lock()
	c.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
	c.mu.Unlock()
}

func (c *ControlLayer) InitVectorClockWithSites(sites []string) {
	c.mu.Lock()
	c.vectorClock = make([]int, len(sites))
	c.nodeIndex = utils.FindIndex(c.GetName(), sites)
	c.vectorClockReady = true
	c.vectorClock[c.nodeIndex] = 0
	c.mu.Unlock()
}

// SendMsgFromApplication is the portal between control layer and application
// layer: the app layer asks the control layer to send a message to the network.
// It is supposed that the application won't send two times the same message,
// so no check if already got the message (=already seen the ID).
// => Is it a receiving action followed by a call to sending action
func (c *ControlLayer) SendApplicationMsg(msg string) error {
	c.mu.Lock()
	// app necessarily has a vector clock if it has started
	recVC := format.RetrieveVectorClock(msg, len(c.vectorClock))
	c.vectorClock = utils.SynchroniseVectorClock(c.vectorClock, recVC, c.nodeIndex)
	c.mu.Unlock()

	c.SendMsg(msg)

	return nil
}

// Send a Message (from this control layer). If is it a propagation, then
// set `fixed_clock` to the same clock as received message to propagate.
func (c *ControlLayer) SendControlMsg(msg_content string, msg_content_type string,
	msg_type string, destination string, fixed_id string,
	sender_name_source string) error {
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
			"sender_name_source", sender_name_source,
			"sender_type", "control",
			"destination", destination,
			"clk", "", // Will be replaced in c.SendMsg
			"vector_clock", "", // Will be replaced in c.SendMsg
			"content_type", msg_content_type,
			"content_value", msg_content))

	c.SendMsg(msg)

	return nil
}

// SendMsg sends a message to the network.
// Param through_channel is used to indicate if the message 
// should be sent through the channel to the application layer ONLY (ie.
// not to the network).
func (c *ControlLayer) SendMsg(msg string, through_channelArgs... bool) {
	through_channel := false
	if len(through_channelArgs) > 0 {
		through_channel = through_channelArgs[0]
	}

	// As this node sends the message, it doens't want to receive
	// a duplicate => add the message ID to ID watcher
	var msg_id_str string = format.Findval(msg, "id", c.GetName())
	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]
	// The sender can also be the app layer, so check for that:
	var msg_sender string = format.Findval(msg, "sender_name_source", c.GetName())
	c.AddNewMessageId(msg_sender, msg_NbMessageSent_str)

	c.mu.Lock()
	if c.vectorClockReady {
		c.vectorClock[c.nodeIndex] += 1 // Incrément de l'horloge vectorielle locale
		msg = format.Replaceval(msg, "vector_clock", utils.SerializeVectorClock(c.vectorClock))
		if len(c.vectorClock) > 7 {
			panic("Vector clock too long")
		}
	}
	c.clk += 1 // Incrément de l'horloge locale
	msg = format.Replaceval(msg, "clk", strconv.Itoa(c.clk))

	if !strings.Contains(msg, "snapshot_id") {
		msg = format.AddFieldToMessage(msg, "snapshot_id", strconv.Itoa(c.nodeMarker))
	}
	c.mu.Unlock()
	if through_channel {
		select {
		case c.channel_to_application <- msg:
			// Message sent successfully
		case <-time.After(100 * time.Millisecond):
			format.Display(format.Format_w(c.GetName(), "SendMsg()", "Channel appears to be blocked"))
		}
	} else {
		format.Msg_send(msg, c.GetName())
	}

	c.mu.Lock()
	c.nbMsgSent = c.nbMsgSent + 1
	c.mu.Unlock()

}

// SendPearDiscovery sends a message asking for pear discovery
// The content_value of the message is the name to which the
// nodes must answer (in this project, is fixed to node id 0)
// 🔥ONLY node whose id is zero will send the pear discovery message.
func (c *ControlLayer) SendPearDiscovery() {
	c.SendControlMsg(c.GetName(), "siteName", "pear_discovery", "control", "", c.GetName())
}

func (c *ControlLayer) SendNeighborDiscovery() {
	c.SendControlMsg(c.GetName(), "siteName", "neighbor_discovery", "control", "", c.GetName())
}

func (c *ControlLayer) SendTreeConstruction() {
	c.mu.Lock()
	c.parentNodeName = c.GetName()
	c.nbExpectedTreeAnswers = len(c.directNeighbors)
	c.mu.Unlock()
	c.SendControlMsg(c.GetName(), "siteName", "tree_blue", "control", "", c.GetName())
}

// When closing, node 0 will send the names it acquired during
// pear discovery, so that all nodes can know which nodes are
// in the network.
// 🔥ONLY node whose id is zero will send this message.
func (c *ControlLayer) ClosePearDiscovery() {
	// Append the current nodes name (node responsible of the pear dis.)
	// as it did not received its own name.
	c.mu.Lock()
	c.knownSiteNames = append(c.knownSiteNames, c.GetName())
	c.nbOfKnownSites += 1
	if c.child.Type() == "verifier" {
		c.knownVerifierNames = append(c.knownVerifierNames, c.child.GetName())
	}
	
	msg_content := strings.Join(c.knownSiteNames, utils.PearD_SITE_SEPARATOR)
	if len(c.knownVerifierNames) > 0 {
		msg_content += utils.PearD_VERIFIER_SEPARATOR + strings.Join(c.knownVerifierNames, utils.PearD_SITE_SEPARATOR)
	}
	
	// Display update nb of site as all other nodes will do when receiving:
	format.Display(format.Format_g(c.GetName(), "ClosePearDis()", "Updated nb sites to " + strconv.Itoa(c.nbOfKnownSites)))

	knownSiteNames := c.knownSiteNames
	c.mu.Unlock()

	// Send final sealing message with control names + verifier names
	c.SendControlMsg(msg_content, "siteNames", "pear_discovery_sealing", "control", "", c.GetName())

	// ✅ Init vector clocks of current control layer & child's
	c.InitVectorClockWithSites(knownSiteNames)
	c.child.InitVectorClockWithSites(knownSiteNames)
}

// Return true if the message's ID is contained with
// one of the controler's ID pairs. If it is not contains,
// it adds it to the interval (see utils.message_watch)
// and return false.
func (c *ControlLayer) SawThatMessageBefore(msg string) bool {
	var msg_id_str string = format.Findval(msg, "id", c.GetName())

	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]

	msg_NbMessageSent, err := utils.MIDFromString(msg_NbMessageSent_str)
	if err != nil {
		format.Display(format.Format_e("SawThatMessageBefore()", c.GetName(), "Error in message id: "+err.Error()))
	}

	var sender_name string = format.Findval(msg, "sender_name_source", c.GetName())

	// Never saw that message before
	c.mu.Lock()
	if c.IDWatcher.ContainsMID(sender_name, msg_NbMessageSent) == false {
		c.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()
	// Saw that message before as it is contained in intervals:
	return true
}


//
//
// func (c *ControlLayer) CheckSnapshotCoherence() {
// 	c.mu.Lock()
//
// 	// Copie des données locales pour traitement sans verrou
// 	snapshots := make(map[string]SnapshotData)
// 	for id, snap := range c.receivedSnapshots {
// 		if !strings.Contains(id, "control") {
// 			snapshots[id] = snap
// 		}
// 	}
// 	c.mu.Unlock()
//
// 	ids := []string{}
// 	for id := range snapshots {
// 		ids = append(ids, id)
// 	}
//
// 	// Cas trivial
// 	if len(ids) <= 1 {
// 		c.SaveSnapshotToCSVThreadSafe(snapshots)
// 		return
// 	}
//
// 	// Vérifier compatibilité
// 	for i := 0; i < len(ids); i++ {
// 		for j := i + 1; j < len(ids); j++ {
// 			if !utils.VectorClockCompatible(snapshots[ids[i]].VectorClock, snapshots[ids[j]].VectorClock) {
// 				format.Display(format.Format_e(
// 					c.GetName(), "CheckSnapshotCoherence()",
// 					"❌ Snapshots incohérents entre "+ids[i]+" et "+ids[j]))
// 				c.SaveSnapshotToCSVThreadSafe(snapshots)
// 				return
// 			}
// 		}
// 	}
//
// 	format.Display(format.Format_g(
// 		c.GetName(), "CheckSnapshotCoherence()",
// 		"✅ Snapshots cohérents entre tous les capteurs"))
//
// 	c.SaveSnapshotToCSVThreadSafe(snapshots)
// }
//
// func (c *ControlLayer) getSensorNames() []string {
// 	sensors := []string{}
// 	for _, name := range c.knownSiteNames {
// 		if strings.HasPrefix(name, "sensor") {
// 			sensors = append(sensors, name)
// 		}
// 	}
// 	return sensors
// }
//
// func (c *ControlLayer) PrintSnapshotsTable() {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
//
// 	fmt.Println()
// 	fmt.Println("📦  Snapshots reçus (" + strconv.Itoa(len(c.receivedSnapshots)) + " capteurs)")
// 	fmt.Println("────────────────────────────────────────────────────────────")
// 	fmt.Printf("| %-15s | %-22s | %-15s |\n", "Capteur", "Horloge vectorielle", "Valeurs")
// 	fmt.Println("────────────────────────────────────────────────────────────")
//
// 	for sensor, snapshot := range c.receivedSnapshots {
// 		vcStr := fmt.Sprintf("%v", snapshot.VectorClock)
// 		valStr := fmt.Sprintf("%v", snapshot.Values)
// 		if len(valStr) > 15 {
// 			valStr = valStr[:12] + "..."
// 		}
// 		fmt.Printf("| %-15s | %-22s | %-15s |\n", sensor, vcStr, valStr)
// 	}
//
// 	fmt.Println("────────────────────────────────────────────────────────────")
// 	fmt.Println()
// }
//
// func (c *ControlLayer) SaveSnapshotToCSV() {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
//
// 	// Nom dynamique basé sur l’horloge Lamport du contrôleur
// 	filename := fmt.Sprintf("snapshot_%d.csv", c.vectorClock[c.nodeIndex])
//
// 	file, err := os.Create(filename)
// 	if err != nil {
// 		fmt.Printf("❌ Erreur création fichier %s: %v\n", filename, err)
// 		return
// 	}
// 	defer file.Close()
//
// 	// Écrire l’en-tête
// 	_, _ = file.WriteString("Capteur,HorlogeVectorielle,Valeurs\n")
//
// 	// Écrire chaque ligne
// 	for sensor, snapshot := range c.receivedSnapshots {
// 		vcStr := strings.ReplaceAll(fmt.Sprint(snapshot.VectorClock), " ", "")
// 		valStr := strings.ReplaceAll(fmt.Sprint(snapshot.Values), " ", "")
// 		line := fmt.Sprintf("%s,\"%s\",\"%s\"\n", sensor, vcStr, valStr)
// 		_, _ = file.WriteString(line)
// 	}
//
// 	fmt.Printf("📁 État global sauvegardé dans %s\n", filename)
// }
//
// func (c *ControlLayer) SaveSnapshotToCSVThreadSafe(snapshots map[string]SnapshotData) {
// 	file, err := os.Create("snapshot.csv")
// 	if err != nil {
// 		log.Println("Erreur création fichier snapshot:", err)
// 		return
// 	}
// 	defer file.Close()
//
// 	writer := csv.NewWriter(file)
// 	defer writer.Flush()
//
// 	writer.Write([]string{"ID", "Vector Clock", "Values"})
//
// 	for id, snap := range snapshots {
// 		writer.Write([]string{
// 			id,
// 			utils.SerializeVectorClock(snap.VectorClock),
// 			strings.Join(utils.Float32SliceToStringSlice(snap.Values), ";"),
// 		})
// 	}
// }
