package node

import (
	"bytes"
	"distributed_system/format"
	"distributed_system/models"
	"distributed_system/utils"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ControlLayer struct {
	mu        sync.RWMutex
	id        string
	nodeType  string
	isRunning bool

	clk              int
	vectorClockReady bool  // true when nbOfKnownSites is set
	vectorClock      []int // Size = nbOfKnownSites when init.
	nodeIndex        int   // position of this node in the vector

	child     Node
	nbMsgSent uint64
	// Seen IDs of all received messages:
	// To help preventing the reading of same msg if it loops
	IDWatcher              *format.MIDWatcher
	channel_to_application chan string

	nbOfKnownSites       int
	knownSiteNames       []string
	knownVerifierNames   []string
	sentDiscoveryMessage bool // To send its name only once
	pearDiscoverySealed  bool
	receivedSnapshots    map[string]SnapshotData

	// For the snapshot algorithm:
	markersReceivedFrom   map[int][]string // map[snapshot_id] => list of nodes that sent back the marker for that snap_id
	nodeMarker            int              // current node's marker id
	subTreeState          GlobalSnapshot   // Buffered, waiting for acquiring all to the send to parent
	snapResponseSent      bool             // true if the snapshot response has been sent to the parent
	nbSnapResponsePending int              // number of snapshot responses pending (= 0 => send our response to parent)

	// For the spanning tree:
	parentNodeName        string   // name of the parent node in the spanning tree
	childrenNames         []string // names of the children nodes in the spanning tree
	directNeighbors       []string
	directNeighborsEnd    bool // true when we know our neighbors (delay of 1s after neighbor discovery)
	nbExpectedTreeAnswers int

	// Network related fields:
	networkLayer *NetworkLayer // Reference to the network layer for sending messages
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
		vectorClock:            []int{},
		clk:                    0,
		nodeIndex:              0,
		child:                  child,
		nbMsgSent:              0,
		IDWatcher:              format.NewMIDWatcher(),
		channel_to_application: make(chan string, 10), // Buffered channel
		nbOfKnownSites:         0,
		sentDiscoveryMessage:   false,
		pearDiscoverySealed:    false,
		knownSiteNames:         make([]string, 0),
		knownVerifierNames:     make([]string, 0),
		receivedSnapshots:      make(map[string]SnapshotData),

		// For the snapshot algorithm:
		markersReceivedFrom: make(map[int][]string),
		nodeMarker:          -1,
		subTreeState:        GlobalSnapshot{SnapshotId: ""},

		// For the spanning tree:
		directNeighbors:       make([]string, 0),
		directNeighborsEnd:    false,
		parentNodeName:        "",
		childrenNames:         make([]string, 0),
		nbExpectedTreeAnswers: 0,
	}
}

func (c *ControlLayer) SetNetworkLayer(networkLayer *NetworkLayer) {
	c.mu.Lock()
	c.networkLayer = networkLayer
	c.mu.Unlock()
}

// Start begins the control operations
func (c *ControlLayer) Start() error {

	for c.networkLayer == nil {
		time.Sleep(time.Duration(1) * time.Second) // Wait for the network layer to be set
	}

	format.Display(format.Format_d(c.GetName(), "Start()", "Starting control layer "+c.GetName()))

	// Notify child that this is its control layer it must talk to.
	c.child.SetControlLayer(c)

	// Msg to application will be send through channel
	go c.child.HandleMessage(c.channel_to_application)

	format.Display_g(c.GetName(), "Start()", "Control layer "+c.GetName()+" is starting its child app layer "+c.child.GetName())
	go c.child.Start()
	format.Display_g(c.GetName(), "Start()", "Child app layer "+c.child.GetName()+" started successfully")
	c.isRunning = true

	// go func() {
	// 	time.Sleep(2 * time.Second)
	// 	if c.id == "6_control" {
	// 		format.Display_g(c.GetName(), "Start()", "Requesting snapshot")
	// 		c.RequestSnapshot()
	// 	}
	// }()

	// select {}
	return nil
}

// HandleMessage processes incoming messages
func (c *ControlLayer) HandleMessage(channel chan string) {

	for msg := range channel {

		format.Display_d(c.GetName(), "HandleMessage()", "Received message of type: "+format.Findval(msg, "type")+" by "+format.Findval(msg, "sender_name_source")+" through node "+format.Findval(msg, "sender_name")+"to destination "+format.Findval(msg, "destination"))
		// It might happen eg. in a bidirectionnal ring.
		// If it is the case (= duplicate) => ignore.
		if c.SawThatMessageBefore(msg) {
			return
		}
		format.Display_d(c.GetName(), "HandleMessage()", "Message is : "+msg)

		// Receiving operations: update the vector clock and the logical clock
		c.mu.Lock()
		if c.vectorClockReady && format.Findval(msg, "vector_clock") != "" { // can be "" for new_node msg
			// Update the vector clock
			recVC := format.RetrieveVectorClock(msg, len(c.vectorClock))
			if c.nodeIndex > len(recVC)-1 {
				format.Display_e(c.GetName(), "HandleMessage", "Received vector clock is smaller than expected. This should not happen.")
				
			} else {
				vectorClock, err := utils.SynchroniseVectorClock(c.vectorClock, recVC, c.nodeIndex)
				if err != nil {
					if len(c.vectorClock) == 0 || len(c.vectorClock) > len(recVC) {
						c.vectorClock = recVC
					}
				} else {
					c.vectorClock = vectorClock
				}
			}

		}
		resClk, _ := strconv.Atoi(format.Findval(msg, "clk"))
		c.clk = utils.Synchronise(c.clk, resClk)
		c.mu.Unlock()

		// var processMessage bool = c.handleSnapshotMsg(msg) // process=True if normal message
		// if !processMessage {
		// 	return
		// }
		// if strings.Contains(msg, "snapshot") {
		// 	c.handleSnapshotMsg(msg)
		// }

		// Extract msg caracteristics
		var msg_destination string = format.Findval(msg, "destination")
		var msg_type string = format.Findval(msg, "type")

		var sender_name_source string = format.Findval(msg, "sender_name_source")

		// Will be used at the end to check if
		// control layer needs to resend the message to all other nodes
		var propagate_msg bool = false

		if msg_destination == "applications" {
			switch msg_type {
			case "new_reading":
				c.SendMsg(msg, true) // Send to child app
				// propagate_msg = true // Now done by network layer
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
					c.SendPearDiscoveryAnswer(msg)
				}
			case "pear_discovery_sealing":
				if !c.pearDiscoverySealed {
					c.HandlePearDiscoverySealing(msg)
				}

			case "tree_blue":
				c.processBlueTree(msg)

			}
		} else if msg_destination == c.GetName() || msg_destination == strings.Split(c.id, "_")[0] { // The msg is only for the current node
			switch msg_type {
			case "logout_announcement":
				format.Display_g(c.GetName(), "HandleMessage", "Message logout_announcement reçu de "+format.Findval(msg, "sender_name_source"))

			case "connect_neighbors":
				connectTo := format.Findval(msg, "connect_to")
				format.Display_g(c.GetName(), "HandleMessage", "connectTo raw value: "+connectTo)

				c.mu.Lock()
				for i, neighbor := range c.directNeighbors {
					if neighbor == format.Findval(msg, "sender_name_source") {
						c.directNeighbors[i] = connectTo
						break
					}
				}
				c.mu.Unlock()

				if c.networkLayer != nil {
					senderIDStr := format.Findval(msg, "sender_id")
					senderID, err1 := strconv.Atoi(senderIDStr)
					connectToInt, err2 := strconv.Atoi(connectTo)
					if err1 != nil || err2 != nil {
						format.Display_e(c.GetName(), "HandleMessage", fmt.Sprintf("Invalid ID conversion: sender_id='%s' err=%v, connect_to='%s' err=%v", senderIDStr, err1, connectTo, err2))
						break
					}
					err := c.networkLayer.ConnectToNeighbor(connectToInt, senderID)
					if err != nil {
						format.Display_e(c.GetName(), "HandleMessage", "Failed to connect to neighbor "+connectTo)
					}
				}

			case "pear_discovery_answer":
				c.HandlePearDiscoveryAnswerFromResponsibleNode(msg)

			case "neighbor_discovery_answer":
				already_discovered := slices.Contains(c.directNeighbors, sender_name_source)
				if !already_discovered {
					c.directNeighbors = append(c.directNeighbors, sender_name_source)
					format.Display(format.Format_d(c.GetName(), "HandleMessage()", "Neighbor discovered: "+sender_name_source))
					c.nbExpectedTreeAnswers += 1
				}

			case "tree_blue": // tree construction message (blue messages from lecture)
				c.processBlueTree(msg)
			case "tree_red": // tree construction message (red messages from lecture)
				c.processRedTree(msg)
			case "new_node":
				format.Display_g(c.GetName(), "HandleMessage()", "New node received has id "+format.Findval(msg, "new_node")+", and its app layer name is "+format.Findval(msg, "new_node_app_name"))
				newPeersStr := format.Findval(msg, "known_peers")
				// The network layer sends peer IDs, but the control layer works with full names.
				// We need to reconstruct the full names from the IDs.
				newPeerIDs := strings.Split(newPeersStr, utils.PearD_SITE_SEPARATOR)
				newPeerNames := make([]string, len(newPeerIDs))
				for i, id := range newPeerIDs {
					// This assumes a consistent naming convention for control layers.
					name := "control (" + id + "_control)"
					newPeerNames[i] = name
				}
				slices.Sort(newPeerNames)

				// --- Get old state from ControlLayer and Child ---
				c.mu.Lock()
				oldSiteNames := c.knownSiteNames
				oldControlVC := c.vectorClock
				c.mu.Unlock()
				oldChildVC := c.child.GetVectorClock()

				// --- Compute new vector clocks ---
				newControlVC := resizeVC(oldControlVC, oldSiteNames, newPeerNames)
				newChildVC := resizeVC(oldChildVC, oldSiteNames, newPeerNames)

				// --- Atomically update ControlLayer state ---
				c.mu.Lock()
				c.knownSiteNames = newPeerNames
				c.nbOfKnownSites = len(newPeerNames)
				c.vectorClock = newControlVC
				c.nodeIndex = utils.FindIndex(c.GetName(), newPeerNames)
				c.mu.Unlock()

				// --- Update Child state ---
				c.child.SetVectorClock(newChildVC, newPeerNames)

				// --- Update verifier list if the new node is a verifier ---
				newNodeAppName := format.Findval(msg, "new_node_app_name")
				if strings.Contains(strings.ToLower(newNodeAppName), "verifier") {
					c.mu.Lock()
					if !slices.Contains(c.knownVerifierNames, newNodeAppName) {
						c.knownVerifierNames = append(c.knownVerifierNames, newNodeAppName)
						slices.Sort(c.knownVerifierNames)
					}
					verifiersStr := strings.Join(c.knownVerifierNames, utils.PearD_SITE_SEPARATOR)
					c.mu.Unlock()

					// Propager la nouvelle liste à la couche application si c'est un vérifieur
					if c.child.Type() == "verifier" {
						msg_to_verifier := format.Msg_format_multi(format.Build_msg_args(
							"id", c.GenerateUniqueMessageID(),
							"type", "pear_discovery_verifier",
							"sender_name_source", c.GetName(),
							"sender_name", c.GetName(),
							"sender_type", "control",
							"destination", c.child.GetName(),
							"content_type", "siteNames",
							"content_value", verifiersStr,
							"clk", "", // mis à jour dans SendMsg
							"vector_clock", "", // mis à jour dans SendMsg
						))
						c.SendMsg(msg_to_verifier, true) // vers application seulement
					}
				}

				format.Display_g(c.GetName(), "HandleMessage()", "Vector clocks resized to size "+strconv.Itoa(len(newPeerNames)))
				propagate_msg = false
			case "joining_configuration":
				newPeersStr := format.Findval(msg, "known_peers")
				// The network layer sends peer IDs, but the control layer works with full names.
				// We need to reconstruct the full names from the IDs.
				newPeerIDs := strings.Split(newPeersStr, utils.PearD_SITE_SEPARATOR)
				newPeerNames := make([]string, len(newPeerIDs))
				for i, id := range newPeerIDs {
					// This assumes a consistent naming convention for control layers.
					newPeerNames[i] = "control (" + id + "_control)"
				}
				slices.Sort(newPeerNames)

				// --- Get old state from ControlLayer and Child ---
				c.mu.Lock()
				oldSiteNames := c.knownSiteNames
				oldControlVC := c.vectorClock
				c.mu.Unlock()
				oldChildVC := c.child.GetVectorClock()

				// --- Compute new vector clocks ---
				newControlVC := resizeVC(oldControlVC, oldSiteNames, newPeerNames)
				newChildVC := resizeVC(oldChildVC, oldSiteNames, newPeerNames)

				// --- Atomically update ControlLayer state ---
				c.mu.Lock()
				c.knownSiteNames = newPeerNames
				c.nbOfKnownSites = len(newPeerNames)
				c.vectorClock = newControlVC
				c.vectorClockReady = true
				c.nodeIndex = utils.FindIndex(c.GetName(), newPeerNames)
				c.mu.Unlock()

				// --- Update Child state ---
				c.child.SetVectorClock(newChildVC, newPeerNames)

				format.Display_g(c.GetName(), "HandleMessage()", "Vector clocks resized to size "+strconv.Itoa(len(newPeerNames)))

				// Now that the node is configured, request the state from a neighbor
				var neighborToAsk string
				for _, name := range newPeerNames {
					if name != c.GetName() {
						neighborToAsk = name
						break
					}
				}
				if neighborToAsk != "" {
					format.Display_g(c.GetName(), "HandleMessage()", "Requesting application state from neighbor "+neighborToAsk)
					c.SendControlMsg("", "empty", "state_request", neighborToAsk, "", c.GetName())

					// Si le nouveau nœud est un vérifieur, demander la liste complète des vérifieurs
					if c.child.Type() == "verifier" && neighborToAsk != "" {
						format.Display_g(c.GetName(), "HandleMessage()", "Requesting verifiers list from neighbor "+neighborToAsk)
						c.SendControlMsg("", "empty", "verifiers_request", neighborToAsk, "", c.GetName())
					}
				}

				propagate_msg = false

			case "state_request":
				// A neighbor is requesting our state.
				format.Display_g(c.GetName(), "HandleMessage()", "Received state request from "+sender_name_source)

				// The new node is requesting our current state
				// We send it back our recent readings
				var buffer bytes.Buffer
				enc := gob.NewEncoder(&buffer)
				err := enc.Encode(c.child.GetApplicationState())
				if err != nil {
					format.Display_e(c.GetName(), "HandleMessage", "Failed to encode state: "+err.Error())
					return
				}

				// Encode the gob binary data into a text-safe base64 string
				stateStr := base64.RawURLEncoding.EncodeToString(buffer.Bytes())

				c.SendControlMsg(stateStr, "recent_readings_state", "state_response", sender_name_source, "", c.GetName())

			case "state_response":
				format.Display_g(c.GetName(), "HandleMessage()", "Received state response from "+sender_name_source)
				stateStr := format.Findval(msg, "content_value")

				// Decode the base64 string back to gob binary data
				stateBytes, err := base64.RawURLEncoding.DecodeString(stateStr)
				if err != nil {
					format.Display_e(c.GetName(), "HandleMessage", "Failed to decode base64 state: "+err.Error())
					format.Display_e(c.GetName(), "HandleMessage", "State: "+stateStr+"; length: "+strconv.Itoa(len(stateStr)))
					return
				}

				buffer := bytes.NewBuffer(stateBytes)
				decoder := gob.NewDecoder(buffer)
				var receivedState map[string][]models.Reading
				if err := decoder.Decode(&receivedState); err != nil {
					format.Display_e(c.GetName(), "HandleMessage", "Failed to decode application state: "+err.Error())
					return
				}

				// Set the application state on the child node
				c.child.SetApplicationState(receivedState)

			case "verifiers_request":
				// A neighbor verifier is requesting the list of verifiers
				verifiersStr := strings.Join(c.knownVerifierNames, utils.PearD_SITE_SEPARATOR)
				c.SendControlMsg(verifiersStr, "verifier_names", "verifiers_response", sender_name_source, "", c.GetName())

			case "verifiers_response":
				// Réponse reçue : mettre à jour la liste puis transmettre au fils vérifieur
				verifiersStr := format.Findval(msg, "content_value")
				var verifiers []string
				if verifiersStr != "" {
					verifiers = strings.Split(verifiersStr, utils.PearD_SITE_SEPARATOR)
				}
				c.mu.Lock()
				c.knownVerifierNames = verifiers
				c.mu.Unlock()

				if c.child.Type() == "verifier" {
					msg_to_verifier := format.Msg_format_multi(format.Build_msg_args(
						"id", c.GenerateUniqueMessageID(),
						"type", "pear_discovery_verifier",
						"sender_name_source", c.GetName(),
						"sender_name", c.GetName(),
						"sender_type", "control",
						"destination", c.child.GetName(),
						"content_type", "siteNames",
						"content_value", verifiersStr,
						"clk", "", // mis à jour dans SendMsg
						"vector_clock", "", // mis à jour dans SendMsg
					))
					c.SendMsg(msg_to_verifier, true)
				}
			case "lock_reply":
				if c.child.Type() == "verifier" {
					// The msg is for the child app layer (which is a verifier)
					c.SendMsg(msg, true) // Send to child through channel
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
			// propagate_msg = true // Propagate to other verifiers
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
				format.Display_g(c.GetName(), "HandleMessage()", "Control: Received lock_release_and_verified_value from "+format.Findval(msg, "sender_name_source"))
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
		msg_propagate_field := format.Findval(msg, "propagation")
		if propagate_msg && msg_propagate_field != "false" { // Check if propagation disabled for this msg
			c.propagateMessage(msg)
		}

	}
}

// resizeVC creates a new vector clock of a new size, preserving the values
// from the old vector clock for sites that still exist.
func resizeVC(oldVC []int, oldSites []string, newSites []string) []int {
	newVC := make([]int, len(newSites))

	// Create a map of old site names to their clock value for efficient lookup
	oldSiteValueMap := make(map[string]int)
	for i, siteName := range oldSites {
		if i < len(oldVC) {
			oldSiteValueMap[siteName] = oldVC[i]
		}
	}

	// Populate the new vector clock
	for i, newSiteName := range newSites {
		if val, ok := oldSiteValueMap[newSiteName]; ok {
			// This site existed before, so copy its old value
			newVC[i] = val
		} else {
			// This is a new site, initialize its clock to 0
			newVC[i] = 0
		}
	}
	return newVC
}

func (c *ControlLayer) SendLogoutAnnouncement() {
	c.mu.Lock()
	c.isRunning = false
	neighbors := make([]string, len(c.directNeighbors))
	copy(neighbors, c.directNeighbors) // copie pour accès hors du lock
	c.mu.Unlock()

	logoutMsg := format.Msg_format_multi(format.Build_msg_args(
		"id", c.GenerateUniqueMessageID(),
		"type", "logout_announcement",
		"sender_name_source", c.GetName(),
		"sender_name", c.GetName(),
		"sender_type", "control",
		"destination", "", // sera remplacé pour chaque voisin
		"content_value", "Node "+c.GetName()+" is disconnecting",
		"propagation", "false",
	))

	for _, neighbor := range neighbors {
		msg := format.Replaceval(logoutMsg, "destination", neighbor)
		format.Display_g(c.GetName(), "SendLogoutAnnouncement", "Envoi du message logout_announcement au voisin "+neighbor)
		c.SendMsg(msg)
	}
}

func (c *ControlLayer) SendConnectNeighbors() {
	if len(c.networkLayer.activeNeighborsIDs) != 2 {
		format.Display_e(c.GetName(), "SendConnectNeighbors", "Expected exactly 2 neighbors")
		return
	}
	leftNeighbor := strconv.Itoa(c.networkLayer.activeNeighborsIDs[0])  //c.directNeighbors[0]
	rightNeighbor := strconv.Itoa(c.networkLayer.activeNeighborsIDs[1]) //c.directNeighbors[1]

	msgToLeft := format.Msg_format_multi(format.Build_msg_args(
		"id", c.GenerateUniqueMessageID(),
		"type", "connect_neighbors",
		"sender_name_source", c.GetName(),
		"sender_name", c.GetName(),
		"sender_type", "control",
		"destination", leftNeighbor,
		"connect_to", rightNeighbor,
		"propagation", "false",
	))

	msgToRight := format.Msg_format_multi(format.Build_msg_args(
		"id", c.GenerateUniqueMessageID(),
		"type", "connect_neighbors",
		"sender_name_source", c.GetName(),
		"sender_name", c.GetName(),
		"sender_type", "control",
		"destination", rightNeighbor,
		"connect_to", leftNeighbor,
		"propagation", "false",
	))

	format.Display_w(c.GetName(), "SendCon", "Sending msg:"+msgToLeft)
	c.SendMsg(msgToLeft)
	c.SendMsg(msgToRight)

	c.networkLayer.SetDown(true)
}

func (c *ControlLayer) NotifyUserLogout() {
	// Pas de fermeture de canal ici, juste notification

	format.Display_g(c.GetName(), "NotifyUserLogout", "ControlLayer notified of User logout")

	// Notifier NetworkLayer
	if c.networkLayer != nil {
		c.networkLayer.NotifyControlLogout()
	}
}
