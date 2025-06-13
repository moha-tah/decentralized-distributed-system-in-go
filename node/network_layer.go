package node

import (
	"bufio"
	"distributed_system/format"
	"distributed_system/utils"
	"fmt"
	"net"
	"time"

	// "os"
	"slices"
	"strconv"
	"strings"
	"sync"
	// "time"
)

// BaseNode implements common functionality for all node types
type NetworkLayer struct {
	id               string
	mu               sync.Mutex
	nodeType         string
	childType        string // Type of the child node (e.g., "user", "verifier"...)
	isRunning        bool
	nbMsgSent        int
	msg_id           int64
	counter          int           // Temporary variable for the vector clock
	vectorClock      []int         // taille = nombre total de noeuds
	vectorClockReady bool          // true après pear_discovery_sealing
	nodeIndex        int           // position de ce node dans le vecteur
	appNode          *Node         // Pointer to the application node
	controlLayer     *ControlLayer // Pointer to the control layer
	channel_to_control chan string // Channel to send messages to the control layer

	// Keep track of the messages received
	IDWatcher *format.MIDWatcher

	// Network-related fields
	listenPort string
	peers      []string
	// activeCon  		[]net.Conn
	activeNeighborsIDs  []int    // List of IDs of active neighbors
	knownPeersIDs       []string // List of known peers (for admission requests), INCLUDES this node
	waitingForAdmission bool     // True if this node is waiting for an admission
	networkIsRing       bool     // True if the network is a ring (true for >= 2 nodes)

	electionInProgress     bool   // True if an election is in progress. Reset to false when election is closed
	electionParentID       int    // Parent node in the election process, if any
	expectedAnswers        int    // Number of expected answers for admission requests
	electedLeader          int    // ID of the elected leader, if any
	nodeRequestingElection string // Name of the node requesting the election, if any
	// waitingConn 		net.Conn	// Connection to the node that is waiting for admission
}

func NewNetworkLayer(id, nodeType string, appNode *Node, controlLayer *ControlLayer, peers_int []string) NetworkLayer {

	id_str, _ := strconv.Atoi(id)
	listenPort := strconv.Itoa(9000 + id_str)

	return NetworkLayer{
		id:               id,
		mu:               sync.Mutex{},
		nodeType:         "network",
		childType:        nodeType,
		isRunning:        false,
		nbMsgSent:        0,
		msg_id:           0,
		counter:          0,
		nodeIndex:        0,
		vectorClockReady: false,
		IDWatcher:        format.NewMIDWatcher(),
		appNode:          appNode,
		controlLayer:     controlLayer,
		channel_to_control: make(chan string, 10), // Buffered channel to send messages to the control layer

		listenPort:         listenPort,
		peers:              peers_int,
		knownPeersIDs:      []string{},
		activeNeighborsIDs: []int{},
		networkIsRing:      false, // Default to false, will be set when joining the network

		//⚠️ Hypothesis: By default, the node is waiting for admission
		waitingForAdmission: true, // Set to true to wait for admission request

		electionParentID: -1, // No parent by default
		expectedAnswers:  -1,
		electedLeader:    -1,
	}
}

// Start the server and client
func (n *NetworkLayer) Start() {
	// WaitGroup to keep the main function running
	var wg sync.WaitGroup

	n.mu.Lock()
	n.isRunning = true
	if len(n.peers) == 0 {
		n.waitingForAdmission = false // No peers, no admission request needed
		// n.controlLayer.SetNetworkLayer(n)
		n.StartControlLayer()
	} else {
		n.waitingForAdmission = true // Set to true to wait for admission request
	}
	n.knownPeersIDs = []string{n.id} // Initialize known peers with this node's name

	n.mu.Unlock()

	// Start server
	wg.Add(1)
	go n.startServer(n.listenPort, &wg)

	// Start clients to peers
	for _, peer := range n.peers {
		wg.Add(1)
		go n.startClient(peer, &wg)
	}

	// go func() {
	// 	// Start control layer once the network layer is ready
	// 	for {
	// 		n.mu.Lock()
	// 		waitingForAdmission := n.waitingForAdmission
	// 		n.mu.Unlock()
	// 		if !waitingForAdmission {
	// 			break // Exit the loop if admission is granted
	// 		}
	// 		time.Sleep(1 * time.Second) // Wait until the admission is granted
	// 	}
	// 	format.Display_network(n.GetName(), "Start()", "Admission granted, starting control layer.")
	// 	n.controlLayer.Start()
	// }()

	wg.Wait()
}

func (n *NetworkLayer) GetName() string {
	return n.nodeType + " (" + n.id + ")"
}
func (n *NetworkLayer) GenerateUniqueMessageID() string {
	id := "network_" + n.id + "_" + strconv.Itoa(int(n.msg_id))
	n.msg_id++
	return id
}
func (n *NetworkLayer) startServer(port string, wg *sync.WaitGroup) {
	defer wg.Done()
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	format.Display_network(n.GetName(), "startServer()", fmt.Sprintf("Listening on port %s", port))

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting:", err)
			continue
		}
		go n.handleConnection(conn)
	}
}

func (n *NetworkLayer) startClient(peer_id_str string, wg *sync.WaitGroup) {
	defer wg.Done()

	peer_id, _ := strconv.Atoi(peer_id_str)
	address := "localhost:" + strconv.Itoa(9000+peer_id)

	format.Display_network(n.GetName(), "startClient()", fmt.Sprintf("Attempting to connect to %s …", address))
	format.Display_network(n.GetName(), "startClient()", fmt.Sprintf("Connected to %s", address))

	// Ask for admission while not admitted
	for {
		n.mu.Lock()
		for _, p := range n.peers {
			p_int, _ := strconv.Atoi(p)
			destination := strconv.Itoa(p_int + 9000)
			msg := format.Build_msg(
				"type", "admission_request",
				"destination", destination,
				"requester", n.id,
				"requester_app_name", n.controlLayer.child.GetName(),
				"requested_connections", strings.Join(n.peers, utils.PearD_SITE_SEPARATOR),
				"vector_clock", strconv.Itoa(n.counter))
			n.SendMessage(msg, p_int)
		}
		n.mu.Unlock()
		format.Display_network(n.GetName(), "startClient()", "Waiting for admission to be granted...")
		time.Sleep(1 * time.Second) // Wait before sending another admission request
		if !n.waitingForAdmission {
			break // Exit the loop if admission is granted
		}
	}
}

func (n* NetworkLayer) StartControlLayer() {
	format.Display_network(n.GetName(), "Start()", "Starting control layer.")
	n.controlLayer.SetNetworkLayer(n)
	// go n.controlLayer.Start()
	n.controlLayer.Start() 
	go n.controlLayer.HandleMessage(n.channel_to_control)
}

// Start a connection to send a message to a destination node.
// Parameters:
// - msg: the message to send
// - dest_id: the ID of the destination node. Use -1 to send to all active connections
// - toCtrlLayerArg: if true, the message is sent to the control layer instead of the network layers
func (n *NetworkLayer) SendMessage(msg string, dest_id int, toCtrlLayerArg ...bool) int {
	toCtrlLayer := false
	if len(toCtrlLayerArg) > 0 && toCtrlLayerArg[0] {
		toCtrlLayer = true
	}

	// ------------- Add message default values --------------
	msg = format.AddOrReplaceFieldToMessage(msg, "sender_id", n.id)
	if format.Findval(msg, "propagation") == "" || format.Findval(msg, "propagation") == "false" {
		msg = format.AddOrReplaceFieldToMessage(msg, "sender_name_source", n.GetName())
		msg = format.AddOrReplaceFieldToMessage(msg, "id", n.GenerateUniqueMessageID())
	} else {
		if format.Findval(msg, "id") == "" {
			msg = format.AddFieldToMessage(msg, "id", n.GenerateUniqueMessageID())
		}
		if format.Findval(msg, "sender_name_source") == "" {
			msg = format.AddFieldToMessage(msg, "sender_name_source", n.GetName())
		}
	}
	msg = format.AddOrReplaceFieldToMessage(msg, "sender_name", n.GetName())

	if format.Findval(msg, "propagation") == "" {
		msg = format.AddFieldToMessage(msg, "propagation", "false")
	}
	if n.vectorClockReady {
		n.vectorClock[n.nodeIndex] += 1
		msg = format.AddFieldToMessage(msg, "vector_clock", utils.SerializeVectorClock(n.vectorClock))
	} else {
		n.counter++
		counter := strconv.Itoa(n.counter)
		if format.Findval(msg, "counter") == "" {
			msg = format.AddFieldToMessage(msg, "counter", counter)
		} else {
			msg = format.Replaceval(msg, "counter", counter)
		}
	}
	// -------------------------------------------------------

	// As this node sends the message, it doens't want to receive
	// a duplicate => add the message ID to ID watcher
	var msg_id_str string = format.Findval(msg, "id")
	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]
	var msg_sender string = format.Findval(msg, "sender_name_source")
	n.AddNewMessageId(msg_sender, msg_NbMessageSent_str)

	if toCtrlLayer {
		// n.controlLayer.HandleMessage(msg) // Pass the message to the control layer
		n.channel_to_control <- msg // Send the message to the control layer through the channel
	} else {
		// Start connection:
		if dest_id == -1 {
			activeNeighborsIDs := n.activeNeighborsIDs // Get the active neighbors IDs
			for _, nid := range activeNeighborsIDs {
				c, _ := net.Dial("tcp", "localhost:"+strconv.Itoa(9000+nid))
				_, err := c.Write([]byte(msg + "\n"))
				if err != nil {
					format.Display_e(n.GetName(), "sendMsg()", "[!] Error sending to "+strconv.Itoa(9000+nid)+": "+err.Error())
					c.Close()
					return 1
				}
				c.Close()
			}
		} else {
			format.Display_network(n.GetName(), "SendMessage()", fmt.Sprintf("Sending message ID %s of type %s to destination ID %d",
				format.Findval(msg, "id"), format.Findval(msg, "type"), dest_id))

			conn, errDial := net.Dial("tcp", "localhost:"+strconv.Itoa(9000+dest_id))
			if errDial != nil {
				format.Display_e(n.GetName(), "sendMsg()", "[!] Error dialing to conn.RemoteAddr(): "+errDial.Error())
				return 1
			}
			_, err := conn.Write([]byte(msg + "\n"))
			if err != nil {
				format.Display_e(n.GetName(), "sendMsg()", "[!] Error sending to conn.RemoteAddr(): "+err.Error())
				conn.Close()
				return 1
			}
			conn.Close()
		}
	}
	n.nbMsgSent++
	return 0
}

// Propagates message to other active neighbors by changing `sender_name` only.
// Doesn't send to the neighbor that has ID `IDtoAvoid`.
// Call this function with mutex already locked.
func (n *NetworkLayer) propagateMessage(msg string, IDtoAvoid string) {
	for _, nid := range n.activeNeighborsIDs {
		if strconv.Itoa(nid) != IDtoAvoid {
			n.SendMessage(msg, nid)
		}
	}
}

func (n *NetworkLayer) handleConnection(conn net.Conn) {
	defer conn.Close()

	n.mu.Lock()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		if n.SawThatMessageBefore(msg) {
			break	
		}

		peer_id_str := format.Findval(msg, "sender_id")

		if n.electionInProgress && !slices.Contains(n.knownPeersIDs, peer_id_str) {
			break // Ignore messages from nodes that are not known when an election is in progress
		}

		// format.Display_network(n.GetName(), "handleConnection()", fmt.Sprintf("Processing message: ID %s of type %s frrom source %s",
		// 	format.Findval(msg, "id"), format.Findval(msg, "type"), format.Findval(msg, "sender_name_source")))

		canPropagateMessage := format.Findval(msg, "propagation") == "true" && !strings.Contains(format.Findval(msg, "type"), "admission")
		// (no propagation for admission messages that are handled separately)

		msg_type := format.Findval(msg, "type")
		if msg_type == "admission_request" && !n.electionInProgress && !n.waitingForAdmission {
			n.handleAdmissionRequest(msg, conn)
		} else if msg_type == "admission_granted" {
			n.handleAdmissionGranted(msg, conn)
		} else if msg_type == "admission_wave_down" {
			n.handleAdmissionWaveDown(msg, conn)
		} else if msg_type == "admission_wave_up" {
			n.handleAdmissionWaveUp(msg, conn)
		} else {
			msg_destination_id, _ := strconv.Atoi(format.Findval(msg, "destination"))
			if msg_destination_id != -1 && strconv.Itoa(msg_destination_id) == n.id {
				// if none of the above, and if the msg destinationi is my id,
				// then I am the destination of the message.
				// Do something with the message
			} else if format.Findval(msg, "destination") == "control" || format.Findval(msg, "destination") == "applications" {
				// it is a message for me.the upper layers. => Pass to control layer
				// go n.controlLayer.HandleMessage(msg)
				n.channel_to_control <- msg // Send the message to the control layer through the channel
			} else if format.Findval(msg, "destination") == n.controlLayer.GetName() {
				// go n.controlLayer.HandleMessage(msg)
				n.channel_to_control <- msg // Send the message to the control layer through the channel
				canPropagateMessage = false
			}
		}

		// Propagate the message to other nodes if needed
		if canPropagateMessage {
			n.propagateMessage(msg, peer_id_str)
		}
		break	
	}
	n.mu.Unlock()
}

// This function is called by the control layer to send a message to the network layer.
// Nothing is done yet (only prints).
func (n *NetworkLayer) MessageFromControlLayer(msg string) {
	format.Display_network(n.GetName(), "MessageFromControlLayer()", "Propagating message of control layer to all active connections.")
	msg = format.AddOrReplaceFieldToMessage(msg, "sender_name_source", n.controlLayer.GetName())
	msg = format.AddOrReplaceFieldToMessage(msg, "propagation", "true") // Set propagation to true
	n.propagateMessage(msg, "-1")
}

// ============================= ADMISSION HANDLING FUNCTIONS ============================

// handleAdmissionRequest processes an admission request message
// For now it only re sponds with a simple acknowledgment
func (n *NetworkLayer) handleAdmissionRequest(msg string, conn net.Conn) {
	senderName := format.Findval(msg, "sender_name")
	requesterID := format.Findval(msg, "requester")
	requestedConnections_str := format.Findval(msg, "requested_connections")
	requestedConnections := strings.Split(requestedConnections_str, utils.PearD_SITE_SEPARATOR)

	if slices.Contains(n.knownPeersIDs, requesterID) {
		return // Ignore admission request if requester is already known (duplicate request)
	}

	// Verify that the requested connections are all known
	for _, connID := range requestedConnections {
		if !slices.Contains(n.knownPeersIDs, connID) {
			return
		}
	}

	format.Display_network(n.GetName(), "handleAdmissionRequest()", "Received admission request from "+senderName+" all requested connections are known: "+requestedConnections_str)
	// Verify that the requested connections are valid and maintains the ring structure
	if (n.networkIsRing && len(requestedConnections) != 2) || (!n.networkIsRing && len(requestedConnections) != 1) {
		format.Display_e(n.GetName(), "handleAdmissionRequest()", "Admission request from "+senderName+" with wrong number of connections, ignoring.")
		return
	}

	if n.electedLeader == -1 { // No leader elected yet, so we become the leader
		format.Display_network(n.GetName(), "handleAdmissionRequest()", "No leader elected yet, starting election process.")
		n.electedLeader, _ = strconv.Atoi(n.id) // Set this node as the leader
		n.nodeRequestingElection = requesterID  // Store the node requesting the election
		n.electionParentID = -1
		n.expectedAnswers = len(n.activeNeighborsIDs) // Reset expected answers to the number of neighbors (no minus one as the node requesting admission is not in activeCon)
		n.electionInProgress = true

		// ---- debug ----
		activeNeiIDs := ""
		for _, id := range n.activeNeighborsIDs {
			activeNeiIDs += strconv.Itoa(id) + ", "
		}
		format.Display_network(n.GetName(), "handleAdmissionRequest()", "Waiting answers from neighbors: "+activeNeiIDs+" (expected answers: "+strconv.Itoa(n.expectedAnswers)+")")
		// ---- debug ----

		expectedAnswers := n.expectedAnswers
		// Start election process
		if expectedAnswers > 0 {
			format.Display_network(n.GetName(), "handleAdmissionRequest()", "No leader elected, trying to become the leader: "+strconv.Itoa(n.electedLeader))
			msg_election := msg
			msg_election = format.Replaceval(msg_election, "type", "admission_wave_down")
			msg_election = format.AddFieldToMessage(msg_election, "leader_id", n.id)
			activeNeighborsIDs := n.activeNeighborsIDs

			for _, id := range activeNeighborsIDs {
				msg_election = format.Replaceval(msg_election, "destination", strconv.Itoa(id))
				n.SendMessage(msg_election, id)
			}
		} else {
			// If no neighbors => de facto leader
			format.Display_network(n.GetName(), "handleAdmissionRequest()", "No neighbors, I am the leader!")
			n.acceptAdmission(msg, conn) // Accept the admission for the requester
		}
	}
}

// handleAdmissionWaveDown processes an admission wave down message
func (n *NetworkLayer) handleAdmissionWaveDown(msg string, conn net.Conn) {
	// Retrieve ID from the sender name
	senderName := format.Findval(msg, "sender_name_source")
	senderID := format.Findval(msg, "sender_id")
	requesterName := format.Findval(msg, "requester")
	rcvLeaderId, err := strconv.Atoi(format.Findval(msg, "leader_id")) // Get the leader ID from the message
	if err != nil {
		format.Display_e(n.GetName(), "handleAdmissionWaveDown()", "Error parsing leader ID: "+err.Error())
		return
	}

	msgToSendAfterOperations := map[int]string{}
	electedLeader := n.electedLeader

	// Don't process admission wave down if the node is already known
	if slices.Contains(n.knownPeersIDs, requesterName) {
		return
	}

	if electedLeader == -1 || rcvLeaderId < electedLeader { // Better leader found (smaller ID)
		previousLeader := n.electedLeader
		n.electedLeader = rcvLeaderId
		n.electionParentID, _ = strconv.Atoi(senderID)
		n.expectedAnswers = len(n.activeNeighborsIDs) - 1 // Reset expected answers to the number of neighbors (minus election parent)
		expectedAnswers := n.expectedAnswers
		format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "Elected new leader: "+strconv.Itoa(n.electedLeader)+" (previous: "+strconv.Itoa(previousLeader)+")")

		if expectedAnswers > 0 {
			// ----------- Send admission wave down to all neighbors (except the sender) ------------
			format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "Sending admission wave down to all neighbors, except "+senderName)
			activeNeighborsIDs := n.activeNeighborsIDs
			for _, nid := range activeNeighborsIDs {
				if strconv.Itoa(nid) != senderID {
					msg_wave_down := format.Replaceval(msg, "sender_name", n.GetName())
					msg_wave_down = format.Replaceval(msg_wave_down, "destination", strconv.Itoa(nid))
					msgToSendAfterOperations[nid] = msg_wave_down
				}
			}
		} else {
			// Send up to the sender
			format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "No neighbors to send admission wave down, sending up to the sender "+senderName)
			msg_wave_up := format.Build_msg(
				"type", "admission_wave_up",
				"destination", senderID,
				"requester", requesterName, // The node requesting admission
				"requester_app_name", format.Findval(msg, "requester_app_name"), 
				"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
				"leader_id", strconv.Itoa(rcvLeaderId),
			)
			senderID_int, _ := strconv.Atoi(senderID)
			msgToSendAfterOperations[senderID_int] = msg_wave_up

		}

	} else {
		format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "Current leader remains: "+strconv.Itoa(n.electedLeader))
		if n.electedLeader == rcvLeaderId {
			msg_wave_up := format.Replaceval(msg, "type", "admission_wave_up")
			msg_wave_up = format.Replaceval(msg_wave_up, "destination", senderID)
			senderID_int, _ := strconv.Atoi(senderID)
			msgToSendAfterOperations[senderID_int] = msg_wave_up
		}
	}
	for nid, msgToSend := range msgToSendAfterOperations {
		n.SendMessage(msgToSend, nid)
	}
}

// handleAdmissionWaveUp processes an admission wave up message
func (n *NetworkLayer) handleAdmissionWaveUp(msg string, conn net.Conn) {
	format.Display_network(n.GetName(), "handleAdmissionWaveUp()", "Received admission wave up message from "+format.Findval(msg, "sender_name"))
	rcvLeaderId, _ := strconv.Atoi(format.Findval(msg, "leader_id"))

	electedLeader := n.electedLeader

	if electedLeader == rcvLeaderId {
		n.expectedAnswers--
		expectedAnswers := n.expectedAnswers
		electedLeader := n.electedLeader
		if expectedAnswers == 0 {
			if strconv.Itoa(electedLeader) == n.id { // We are the elected leader
				// If we are the elected leader and we received all expected answers,
				// we can finalize the admission process
				format.Display_g(n.GetName(), "handleAdmissionWaveUp()", "✅ All neighbors have acknowledged, admission process completed.")
				n.acceptAdmission(msg, conn) // Accept the admission for the requester
				return
			} else {
				format.Display_network(n.GetName(), "handleAdmissionWaveUp()", fmt.Sprintf("Received admission wave up, sending to parent %d", n.electionParentID))
				msg_wave_up := format.Build_msg(
					"type", "admission_wave_up",
					"destination", "network",
					"requester", format.Findval(msg, "requester"), // The node requesting admission
					"requester_app_name", format.Findval(msg, "requester_app_name"), 
					"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
					"leader_id", strconv.Itoa(electedLeader),
				)
				n.SendMessage(msg_wave_up, n.electionParentID)
				return
			}
		} else {
			format.Display_network(n.GetName(), "handleAdmissionWaveUp()", fmt.Sprintf("Received admission wave up, but still waiting for %d more answers", expectedAnswers))
		}
	} else {
		format.Display_w(n.GetName(), "handleAdmissionWaveUp()", "Received admission wave up for a different leader: "+strconv.Itoa(rcvLeaderId)+". Ignoring.")
	}
}

// acceptAdmission sends an admission response to all active connections
// for all node to reset their leader and accept the new node
func (n *NetworkLayer) acceptAdmission(msg string, conn net.Conn) {
	slices.Sort(n.knownPeersIDs)
	knownPeersIDs := n.knownPeersIDs

	requester := format.Findval(msg, "requester")    // The node requesting acceptAdmission
	knownPeersIDs = append(knownPeersIDs, requester) // Add the requester to known peers

	msg_content := strings.Join(knownPeersIDs, utils.PearD_SITE_SEPARATOR)

	// Prepare admission response message
	responseMsg := format.Build_msg(
		"type", "admission_granted",
		"destination", "network",
		"requester", requester, // The node requesting admission
		"requester_app_name", format.Findval(msg, "requester_app_name"), 
		"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
		"known_peers", msg_content, // List of known peers
	)

	format.Display_network(n.GetName(), "acceptAdmission()", "Sent admission response to all active connections.")

	// Maybe I am the node connected to the new node! If this is the case, I need to accept the new node
	n.handleAdmissionGranted(responseMsg, conn) // Call handleAdmissionGranted to process the admission granted message
}

// handleAdmissionGranted processes an admission granted message
// Two cases:
// 1. For the requesting node, it means it has been admitted and can start its activity.
// 2. For the node connected to the requesting node, it means it can now accept the new node
func (n *NetworkLayer) handleAdmissionGranted(msg string, conn net.Conn) {

	senderID_int, _ := strconv.Atoi(format.Findval(msg, "sender_id"))

	msgToSendAfterOperations := map[int]string{}
	msgToSendAfterOperationsToControl := []string{}

	waitingForAdmission := n.waitingForAdmission
	requestedConnections := format.Findval(msg, "requested_connections")
	requesterID_int, _ := strconv.Atoi(format.Findval(msg, "requester"))

	if waitingForAdmission {
		n.activeNeighborsIDs = append(n.activeNeighborsIDs, senderID_int) // Add the new connection to active connections
		if len(n.activeNeighborsIDs) == len(n.peers) {
			n.waitingForAdmission = false // All peers are connected, no more waiting for admission
			format.Display_network(n.GetName(), "handleAdmissionGranted()", "All peers are connected, no more waiting for admission.")

			n.StartControlLayer()

			// Notify the control layer that we are joining the configuration
			notif_msg := format.Build_msg(
				"type", "joining_configuration",
				"destination", n.controlLayer.GetName(), // Send to control layer
				"id_of_neighbors", format.Findval(msg, "requested_connections"), // The requested connections
				"known_peers", strings.Join(n.knownPeersIDs, utils.PearD_SITE_SEPARATOR), // List of known peers
			)
			msgToSendAfterOperationsToControl = append(msgToSendAfterOperationsToControl, notif_msg)
		} else {
			n.waitingForAdmission = true // Still waiting for admission from other peers
		}
		format.Display_network(n.GetName(), "handleAdmissionGranted()", "Admission granted! I am now connected to: "+strconv.Itoa(senderID_int))

		// Reset list of known peers with name contained in the message
		sites_parts := strings.Split(format.Findval(msg, "known_peers"), utils.PearD_SITE_SEPARATOR)
		n.knownPeersIDs = []string{}
		n.knownPeersIDs = append(n.knownPeersIDs, sites_parts...)
	} else {

		if strings.Contains(requestedConnections, n.id) {
			// I am connected to the new node => send admission granted message to the new node
			// only if I don't know it already
			if !slices.Contains(n.knownPeersIDs, format.Findval(msg, "requester")) {
				granted_msg := format.Replaceval(msg, "propagation", "false")
				// n.SendMessage(granted_msg, requesterID_int)
				msgToSendAfterOperations[requesterID_int] = granted_msg
			}
			// 💡 I am already in the network. The new node wants to connect to me.
			// As this is a ring, the new node also connect to another node. I thus need to
			// stop my connection with this other node to maintain the ring network.
			// So I keep as neighbors the requester and the node I am connected to that
			// will not be connected to the new node.
			// But ONLY do that if we were three or more nodes, before the new node arrived,
			// as if we are only two, I need to keep the connection with the other node.
			knownPeersIDs := n.knownPeersIDs

			if len(knownPeersIDs) >= 3 { // A ring network was already existing before the new node arrived.
				requesterId, _ := strconv.Atoi(format.Findval(msg, "requester"))
				activeNeighborsIDs := []int{requesterId}
				for _, nid := range n.activeNeighborsIDs {
					if !strings.Contains(requestedConnections, strconv.Itoa(nid)) {
						activeNeighborsIDs = append(activeNeighborsIDs, nid)
						break // Only two neighbors in a ring network
					}
				}
				n.activeNeighborsIDs = activeNeighborsIDs
			} else {
				n.activeNeighborsIDs = append(n.activeNeighborsIDs, requesterID_int)
			}

			// --------- debug -----------
			kept_nids := ""
			for _, nid := range n.activeNeighborsIDs {
				kept_nids += strconv.Itoa(nid) + ", "
			}
			format.Display_network(n.GetName(), "handleAdmissionGranted()", "The new node ("+format.Findval(msg, "requester")+") is connected to me. Kept neighbors: "+kept_nids)
			// ---------------------------

		}

		n.knownPeersIDs = append(n.knownPeersIDs, format.Findval(msg, "requester"))

		// Propagate admission_granted to other nodes except the requester and the node
		// from with I am receiving the message.
		for _, nid := range n.activeNeighborsIDs {
			if nid != requesterID_int && nid != senderID_int {
				responseMsg := format.Build_msg(
					"type", "admission_granted",
					"destination", strconv.Itoa(nid),
					"propagation", "true",
					"requester", format.Findval(msg, "requester"),
					"requester_app_name", format.Findval(msg, "requester_app_name"), 
					"requested_connections", format.Findval(msg, "requested_connections"),
					"known_peers", format.Findval(msg, "known_peers"),
				)
				msgToSendAfterOperations[nid] = responseMsg
			}
		}

		active_nids := []string{}
		for _, nid := range n.activeNeighborsIDs {
			active_nids = append(active_nids, strconv.Itoa(nid))
		}
		notif_msg := format.Build_msg(
			"type", "new_node",
			"destination", n.controlLayer.GetName(), // Send to control layer
			"new_node", format.Findval(msg, "requester"), // The new node that has been admitted
			"new_node_app_name", format.Findval(msg, "requester_app_name"), 
			"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
			"known_peers", strings.Join(n.knownPeersIDs, utils.PearD_SITE_SEPARATOR), // List of known peers
			"id_of_neighbors", strings.Join(active_nids, utils.PearD_SITE_SEPARATOR), // IDs of active neighbors
		)
		msgToSendAfterOperationsToControl = append(msgToSendAfterOperationsToControl, notif_msg)
	}

	n.electedLeader = -1
	n.electionParentID = -1
	n.expectedAnswers = -1        // Reset expected answers
	n.nodeRequestingElection = "" // Reset the node requesting the election
	n.networkIsRing = true
	n.electionInProgress = false

	format.Display_network(n.GetName(), "handleAdmissionGranted()", "Known peers: "+strings.Join(n.knownPeersIDs, ", "))

	// --------- debug -----------
	active_nids := ""
	for _, nid := range n.activeNeighborsIDs {
		active_nids += strconv.Itoa(nid) + ", "
	}
	format.Display_network(n.GetName(), "handleAdmissionGranted()", "Active neighbors: "+active_nids)
	// ----------------------------

	for nid, msgToSend := range msgToSendAfterOperations {
		n.SendMessage(msgToSend, nid)
	}
	for _, msgToSend := range msgToSendAfterOperationsToControl {
		n.SendMessage(msgToSend, -1, true) // Send to control layer
	}
}

// ============================ ID WATCHER FUNCTIONS ============================
// Return true if the message's ID is contained with
// one of the network's ID pairs. If it is not contains,
// it adds it to the interval (see format.message_watch)
// and return false.
// USAGE: prevent from receiving the same message twice
func (n *NetworkLayer) SawThatMessageBefore(msg string) bool {
	var msg_id_str string = format.Findval(msg, "id")
	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]
	msg_NbMessageSent, err := format.MIDFromString(msg_NbMessageSent_str)

	if err != nil {
		format.Display(format.Format_e("SawThatMessageBefore()", n.GetName(), "Error in message id: "+err.Error()))
	}

	var sender_name string = format.Findval(msg, "sender_name_source")

	// Never saw that message before
	if !n.IDWatcher.ContainsMID(sender_name, msg_NbMessageSent) {
		n.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
		return false
	}
	// Saw that message before as it is contained in intervals:
	return true
}

// AddNewMessageId adds an entry in the seenIDs to remember
// that the network layer saw this message.
func (n *NetworkLayer) AddNewMessageId(sender_name string, MID_str string) {
	msg_NbMessageSent, err := format.MIDFromString(MID_str)
	if err != nil {
		format.Display(format.Format_e("AddNewMessageID()", n.GetName(), "Error in message id: "+err.Error()))
	}

	n.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
}
