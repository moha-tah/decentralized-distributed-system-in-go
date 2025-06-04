package node

import (
	"bufio"
	"distributed_system/format"
	"distributed_system/utils"
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BaseNode implements common functionality for all node types
type NetworkLayer struct {
	id        	string
	mu		sync.Mutex
	nodeType  	string
	childType	string	// Type of the child node (e.g., "user", "verifier"...)
	isRunning 	bool
	nbMsgSent	int
	counter		int 		// Temporary variable for the vector clock
	vectorClock	[]int 		// taille = nombre total de noeuds
	vectorClockReady bool 		// true après pear_discovery_sealing
	nodeIndex   	int   		// position de ce node dans le vecteur
	appNode  	*Node 	// Pointer to the application node
	controlLayer	*ControlLayer // Pointer to the control layer

	// Keep track of the messages received
	IDWatcher *format.MIDWatcher

	// Network-related fields
	listenPort	string
	peers      	[]string
	activeCon  	[]net.Conn
	waitingForAdmission bool 	// True if this node is waiting for an admission
	knownPeers 	[]string 	// List of known peers (for admission requests), INCLUDES this node
	networkIsRing	bool 		// True if the network is a ring

	electionParent 	net.Conn 	// Parent node in the election process, if any
	expectedAnswers int 		// Number of expected answers for admission requests
	electedLeader 	string 		// Name of the elected leader, if any
	nodeRequestingElection string	// Name of the node requesting the election, if any 
	waitingConn 	net.Conn	// Connection to the node that is waiting for admission
}

func NewNetworkLayer(id, nodeType string, appNode *Node, controlLayer *ControlLayer, listenPort string, peers_int []string) NetworkLayer {

	// Convert peers ID (eg. 1) to localhost address (eg. localhost:9001)
	peers := []string{}
	for _, pint := range peers_int {
		pint, err := strconv.Atoi(pint)
		if err != nil {
			format.Display_e("", "", "Wrong peer integer value, failed to parse to int: " + err.Error())
		}
		peers = append(peers, "localhost:" + strconv.Itoa(9000 + pint))
	}

	return NetworkLayer{
		id:        id,
		mu:        sync.Mutex{},
		nodeType:  "network",
		childType: nodeType, 
		isRunning: false,
		nbMsgSent: 0,
		counter: 0,
		nodeIndex: 0,
		vectorClockReady: false,
		IDWatcher: format.NewMIDWatcher(),
		appNode: appNode,
		controlLayer: controlLayer,

		listenPort: listenPort,
		peers:     peers,
		knownPeers: []string{},
		networkIsRing: false, // Default to false, will be set when joining the network

		//⚠️ Hypothesis: By default, the node is waiting for admission
		waitingForAdmission: true, // Set to true to wait for admission request

		electionParent: nil, // No parent by default
		expectedAnswers: -1,
		electedLeader: "",
	}
}


// Start the server and client
func (n* NetworkLayer) Start() {
	// WaitGroup to keep the main function running
	var wg sync.WaitGroup

	n.mu.Lock()
	n.isRunning = true
	if len(n.peers) == 0 {
		n.waitingForAdmission = false // No peers, no admission request needed
	} else {
		n.waitingForAdmission = true // Set to true to wait for admission request
	}
	n.knownPeers = []string{n.GetName()} // Initialize known peers with this node's name

	if len(n.peers) == 0 {
		n.waitingForAdmission = false // No peers, no admission request needed
	}

	n.mu.Unlock()

	// Start server
	wg.Add(1)
	go n.startServer(n.listenPort, &wg)

	// Start clients to peers
	for _, peer := range n.peers {
		wg.Add(1)
		go n.startClient(peer, &wg)
	}

	go func() {
		// Start control layer once the network layer is ready 
		for {
			n.mu.Lock()
			waitingForAdmission := n.waitingForAdmission
			n.mu.Unlock()
			if !waitingForAdmission {
				break // Exit the loop if admission is granted
			}
			time.Sleep(1 * time.Second) // Wait until the admission is granted
		}
		format.Display_network(n.GetName(), "Start()", "Admission granted, starting control layer.")
		n.controlLayer.Start(n) // Start the control layer
	}()

	wg.Wait()
}

func (n* NetworkLayer) GetName() string {
	return n.nodeType + " (" + n.id + ")"
}
func (n* NetworkLayer) GenerateUniqueMessageID() string {
	n.mu.Lock()
	id :=  "control_" + n.id + "_" + strconv.Itoa(int(n.nbMsgSent))
	n.mu.Unlock()
	return id
}
func (n* NetworkLayer) startServer(port string, wg *sync.WaitGroup) {
	defer wg.Done()
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	fmt.Println("[*] Listening on port", port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting:", err)
			continue
		}
		go n.handleConnection(conn, false)
	}
}


func (n* NetworkLayer) SendMessage(msg string, conn* net.Conn) int {
	// Add message default values
	if format.Findval(msg, "propagation") == "" {
		msg = format.AddFieldToMessage(msg, "propagation", "false")
	} 
	if format.Findval(msg, "sender_name_source") == "" {
		msg = format.AddFieldToMessage(msg, "sender_name_source", n.GetName())
	}
	n.mu.Lock()
	if n.vectorClockReady {
		n.vectorClock[n.nodeIndex] += 1
		msg = format.AddFieldToMessage(msg, "vector_clock", utils.SerializeVectorClock(n.vectorClock))
	} else  {
		n.counter++
		counter := strconv.Itoa(n.counter)
		if format.Findval(msg, "counter") == "" {
			msg = format.AddFieldToMessage(msg, "counter", counter)
		} else {
			msg = format.Replaceval(msg, "counter", counter)
		}
	}
	n.mu.Unlock()


	// As this node sends the message, it doens't want to receive
	// a duplicate => add the message ID to ID watcher
	var msg_id_str string = format.Findval(msg, "id")
	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]
	// The sender can also be the app layer, so check for that:
	var msg_sender string = format.Findval(msg, "sender_name_source")
	n.AddNewMessageId(msg_sender, msg_NbMessageSent_str)

	fmt.Println("Sending: " + msg)
	if (*conn) == nil {
		// Send to all active connections:
		n.mu.Lock()
		activeCons := n.activeCon
		n.mu.Unlock()
		for _, c := range activeCons {
			_, err := c.Write([]byte(msg + "\n"))
			if err != nil {
				format.Display_e(n.GetName(), "sendMsg()", "[!] Error sending to conn.RemoteAddr(): " + err.Error())
				return 1
			}
		}
	} else {
		_, err := (*conn).Write([]byte(msg + "\n"))
		if err != nil {
			format.Display_e(n.GetName(), "sendMsg()", "[!] Error sending to conn.RemoteAddr(): " + err.Error())
			return 1
		}
	}
	n.mu.Lock()
	n.nbMsgSent++
	n.mu.Unlock()
	return 0
}

func (n* NetworkLayer) startClient(address string, wg *sync.WaitGroup) {
	defer wg.Done()

	format.Display_network(n.GetName(), "startClient()", fmt.Sprintf("Attempting to connect to %s …", address))
	conn, err := net.Dial("tcp", address)
	for err != nil {
		format.Display_e(n.GetName(), "startClient()", fmt.Sprintf("Could not connect to %s: %v. Retrying in 2s…", address, err))
		time.Sleep(2 * time.Second)
		conn, err = net.Dial("tcp", address)
	}

	fmt.Printf("[*] ← Connected to %s\n", address)
	// Launch a reader so we can handle any incoming from this peer
	go n.handleConnection(conn, true) 

	// Ask for admission while not admitted
	for {
		msg := format.Build_msg(
			"sender_name", n.GetName(),
			"type", "admission_request",
			"id", n.GenerateUniqueMessageID(),
			"destination", "all",
			"requester", n.GetName(),
			"requested_connections", strings.Join(n.peers, utils.PearD_SITE_SEPARATOR),
			"vector_clock", strconv.Itoa(n.counter))
		if n.SendMessage(msg, &conn) == 1 {
			format.Display_e(n.GetName(), "startClient()", "Error sending admission request in network.sendMessage()")
			conn.Close()
		}
		format.Display_network(n.GetName(), "startClient()", "Waiting for admission to be granted...")
		time.Sleep(1 * time.Second) // Wait before sending another admission request
		if n.waitingForAdmission == false {
			format.Display_network(n.GetName(), "startClient()", "Admission granted, starting activity.")
			break // Exit the loop if admission is granted
		}
	}
}

func (n* NetworkLayer) handleConnection(conn net.Conn, isClient bool) {
	defer conn.Close()
	
	n.mu.Lock()
	n.waitingConn = conn
	n.mu.Unlock()

	remoteAddr := conn.RemoteAddr().String()
	fmt.Println("[*] Connection from/to", remoteAddr)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		format.Display_network(n.GetName(), "handleConnection()", "Message from "+remoteAddr+": "+msg)
		if n.SawThatMessageBefore(msg) {
			continue
		}

		msg_destination := format.Findval(msg, "destination")
		msg_destination_id, _ := utils.ExtractIDFromName(msg_destination)
		msg_type := format.Findval(msg, "type")
		
		if msg_type == "admission_request" {
			n.handleAdmissionRequest(msg, conn)
		} else if msg_type== "admission_granted" {
			n.handleAdmissionGranted(msg, conn)
		} else if msg_type == "admission_rejected" {
			format.Display_e(n.GetName(), "handleConnection()", "Admission request rejected by "+format.Findval(msg, "sender_name"))
			os.Exit(1) // Exit the program if admission is rejected
		} else if msg_type == "admission_wave_down" {
			n.handleAdmissionWaveDown(msg, conn)
		} else if msg_type == "admission_wave_up" {
			n.handleAdmissionWaveUp(msg, conn)
		} else if msg_type == "close_connection" {
			break // We were asked to close the connection, so we break the loop
		} else {
			if msg_destination_id != -1 && strconv.Itoa(msg_destination_id) == n.id {
				// if none of the above, it is a message for the upper layers
				//Pass to control layer
				go n.controlLayer.HandleMessage(msg)
			}
		}

		// Propagate the message to other nodes if needed
		msg_propagate_field := format.Findval(msg, "propagation")
		if msg_propagate_field != "false" { // Check if propagation disabled for this msg
			n.mu.Lock()
			activeCons := n.activeCon
			n.mu.Unlock()
			propagate_to := ""
			for _, c := range activeCons {
				if c != conn { // Do not send back to where the msg is coming from
					propagate_msg := format.Replaceval(msg, "sender_name", n.GetName())
					n.SendMessage(propagate_msg, &c)
					propagate_to += c.RemoteAddr().String() + ", "
				}
			}
			if propagate_to != "" {
				format.Display_g(n.GetName(), "", "Propagating to active connections: " + propagate_to)
			}
		}

	}
	if err := scanner.Err(); err != nil {
		format.Display_e(n.GetName(), "handleCon()", fmt.Sprintf("Read error from %s: %v\n", remoteAddr, err))
	} else {
		format.Display_w(n.GetName(), "handleCon()", fmt.Sprintf("Connection closed by %s\n", remoteAddr))
	}

	// Remove the connection from active connections
	n.mu.Lock()
	index := -1
	for i, c := range n.activeCon {
		if c == conn {
			index = i
			break
		}
	}
	if index != -1 {
		n.activeCon = slices.Delete(n.activeCon, index, index+1)
	}
	n.mu.Unlock()
}

// ============================= ADMISSION HANDLING FUNCTIONS ============================

// handleAdmissionRequest processes an admission request message
// For now it only responds with a simple acknowledgment
func (n* NetworkLayer) handleAdmissionRequest(msg string, conn net.Conn) {
	senderName := format.Findval(msg, "sender_name")
	requesterName := format.Findval(msg, "requester")
	requestedConnections_str := format.Findval(msg, "requested_connections")
	format.Display_network(n.GetName(), "handleAdmissionRequest()", "Received admission request from "+senderName+" for "+requestedConnections_str)
	requestedConnections := strings.Split(requestedConnections_str, utils.PearD_SITE_SEPARATOR)
	format.Display_network(n.GetName(), "handleAdmissionRequest()", "Received admission request from "+senderName)

	n.mu.Lock()
	knownPeers := n.knownPeers 
	networkIsRing := n.networkIsRing // Check if the network is a ring
	n.mu.Unlock()
	if slices.Contains(knownPeers, requesterName) {
		format.Display_w(n.GetName(), "handleAdmissionRequest()", "Admission request from "+requesterName+" already known, ignoring.")
		return // Ignore admission request if requester is already known (duplicate request)
	} else {
		format.Display_network(n.GetName(), "handleAdmissionRequest()", "Will process admission request from "+senderName)
	}


	if (networkIsRing && len(requestedConnections) != 2) || (!networkIsRing && len(requestedConnections) != 1) {
		format.Display_e(n.GetName(), "handleAdmissionRequest()", "Admission request from "+senderName+" with only one requested connection, ignoring. To continue the ring network, please connect to two nodes.")
		msg_reject := format.Build_msg(
			"sender_name", n.GetName(),
			"type", "admission_rejected",
			"id", n.GenerateUniqueMessageID(),
			"destination", "network")
		n.SendMessage(msg_reject, &conn) // Send rejection message back to the requester
		return
	}
			

	if n.electedLeader == "" {
		format.Display_network(n.GetName(), "handleAdmissionRequest()", "No leader elected yet, starting election process.")
		// No leader elected yet, so we become the leader
		n.mu.Lock()
		n.electedLeader = n.GetName() // Set this node as the leader
		n.nodeRequestingElection = requesterName // Store the node requesting the election
		n.electionParent = nil
		n.expectedAnswers = len(n.activeCon) // Reset expected answers to the number of neighbors (no minus one as the node requesting admission is not in activeCon)
		activeCons := ""
		for _, c := range n.activeCon {
			activeCons += c.RemoteAddr().String() + ", "
		}
		format.Display_network(n.GetName(), "handleAdmissionRequest()", "Waiting answers from neighbors: " + activeCons + " (expected answers: " + strconv.Itoa(n.expectedAnswers) + ")")
		expectedAnswers := n.expectedAnswers
		n.mu.Unlock()
		// Start election process
		if expectedAnswers > 0 {
			format.Display_network(n.GetName(), "handleAdmissionRequest()", "No leader elected, trying to become the leader: "+n.electedLeader)
			msg_election := format.Build_msg(
				"sender_name", n.GetName(),
				"type", "admission_wave_down",
				"id", n.GenerateUniqueMessageID(),
				"destination", "network",
				"requester", requesterName, // The node requesting admission
				"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
				"leader_name", n.GetName(), // Set the leader name to this node
				)
			for _, c := range n.activeCon {
				if c != conn { // Do not send back to where the msg is coming from (ie. the node requesting admission)
					n.SendMessage(msg_election, &c)
				}
			}
		} else {
			// If no neighbors => de facto leader
			format.Display_network(n.GetName(), "handleAdmissionRequest()", "No neighbors, I am the leader!")
			n.acceptAdmission(msg ,conn) // Accept the admission for the requester
		}
	} else {
		format.Display_network(n.GetName(), "handleAdmissionRequest()", "Already have a leader: "+n.electedLeader)
		n.mu.Lock()
		if n.nodeRequestingElection == requesterName {
			// I received an admission request but I already have a leader. This occurs
			// when the new node is connected to multiple nodes at the same time. The new node 
			// also asked for admission to the other nodes, which already propagated their leader to me.
			// I need to remember that I am also connected to this new node :
			n.waitingConn = conn // Store the connection for later use
		}
		n.mu.Unlock()
	}
}

// handleAdmissionWaveDown processes an admission wave down message
func (n* NetworkLayer) handleAdmissionWaveDown(msg string, conn net.Conn) {
	// Retrieve ID from the sender name
	senderName := format.Findval(msg, "sender_name")
	leaderName := format.Findval(msg, "leader_name")
	requesterName := format.Findval(msg, "requester")
	newLeader, err := utils.UpdateLeader(n.electedLeader, leaderName)
	if err != nil {
		format.Display_e(n.GetName(), "handleAdmissionWaveDown()", "Error updating leader: "+err.Error())
		return
	}

	if newLeader != n.electedLeader { // Better leader found (smaller ID)
		// If we don't have an elected leader or the new sender ID is smaller,
		// we update the elected leader
		n.mu.Lock()
		n.electedLeader = newLeader // Update the elected leader
		n.electionParent = conn // Update the parent to the new leader
		n.expectedAnswers = len(n.activeCon) -1 // Reset expected answers to the number of neighbors (minus election parent)
		expectedAnswers := n.expectedAnswers
		n.mu.Unlock()
		format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "Elected new leader: "+newLeader)

		if expectedAnswers > 0 {
			// Send admission wave down to all neighbors (except the sender)
			format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "Sending admission wave down to all neighbors, except "+ senderName)
			n.mu.Lock()
			activeCons := n.activeCon
			n.mu.Unlock()
			for _, c := range activeCons {
				if c != conn { // Do not send back to where the msg is coming from
					msg_wave_down := format.Replaceval(msg, "sender_name", n.GetName())
					n.SendMessage(msg_wave_down, &c)
				}
			}
		} else {
			// Send up to the sender
			format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "No neighbors to send admission wave down, sending up to the sender "+senderName)
			msg_wave_up := format.Build_msg(
				"sender_name", n.GetName(),
				"type", "admission_wave_up",
				"id", n.GenerateUniqueMessageID(),
				"destination", "network",
				"requester", requesterName, // The node requesting admission
				"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
				"leader_name", newLeader, // Set the leader name to the new leader
				)
			n.SendMessage(msg_wave_up, &conn)

		}

	} else {
		format.Display_network(n.GetName(), "handleAdmissionWaveDown()", "Current leader remains: "+n.electedLeader)
		if n.electedLeader == newLeader {
			msg_wave_up := format.Replaceval(msg, "sender_name", n.GetName())
			msg_wave_up = format.Replaceval(msg_wave_up, "type", "admission_wave_up")
			n.SendMessage(msg_wave_up, &conn)
		}
	}
}

// handleAdmissionWaveUp processes an admission wave up message
func (n* NetworkLayer) handleAdmissionWaveUp(msg string, conn net.Conn) {
	format.Display_network(n.GetName(), "handleAdmissionWaveUp()", "Received admission wave up message from "+format.Findval(msg, "sender_name"))
	leaderName := format.Findval(msg, "leader_name")
	if n.electedLeader == leaderName {
		n.mu.Lock()
		n.expectedAnswers--
		expectedAnswers := n.expectedAnswers
		n.mu.Unlock()
		if expectedAnswers == 0 {
			if n.electedLeader == n.GetName() {
				// If we are the elected leader and we received all expected answers,
				// we can finalize the admission process
				format.Display_g(n.GetName(), "handleAdmissionWaveUp()", "✅ All neighbors have acknowledged, admission process completed.")
				n.acceptAdmission(msg, conn) // Accept the admission for the requester
			} else {
				format.Display_network(n.GetName(), "handleAdmissionWaveUp()", fmt.Sprintf("Received admission wave up, sending to parent %s", n.electionParent.RemoteAddr().String()))
				msg_wave_up := format.Build_msg(
					"sender_name", n.GetName(),
					"type", "admission_wave_up",
					"id", n.GenerateUniqueMessageID(),
					"destination", "network",
					"requester", format.Findval(msg, "requester"), // The node requesting admission
					"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
					"leader_name", n.electedLeader, // Set the leader name to the current elected leader
					)
				n.SendMessage(msg_wave_up, &n.electionParent)
			}
		} else {
			format.Display_network(n.GetName(), "handleAdmissionWaveUp()", fmt.Sprintf("Received admission wave up, but still waiting for %d more answers", expectedAnswers))
		}
	} else {
		format.Display_w(n.GetName(), "handleAdmissionWaveUp()", "Received admission wave up for a different leader: "+leaderName+" (current leader: "+n.electedLeader+")")
	}
}

// acceptAdmission sends an admission response to all active connections
// for all node to reset their leader and accept the new node
func (n* NetworkLayer) acceptAdmission(msg string, conn net.Conn) {
	n.mu.Lock()
	slices.Sort(n.knownPeers)
	knownPeers := n.knownPeers
	n.mu.Unlock()

	requester := format.Findval(msg, "requester") // The node requesting acceptAdmission
	knownPeers = append(knownPeers, requester) // Add the requester to known peers 
	// The new node is NOT added in mutex, as it is not yet in the active connections.
	// It will be done in handleAdmissionGranted().
	msg_content := strings.Join(knownPeers, utils.PearD_SITE_SEPARATOR)

	// Prepare admission response message
	responseMsg := format.Build_msg(
		"sender_name", n.GetName(),
		"type", "admission_granted",
		"id", n.GenerateUniqueMessageID(),
		"destination", "network",
		"propagation", "true",
		"requester", requester, // The node requesting admission
		"requested_connections", format.Findval(msg, "requested_connections"), // The requested connections
		"known_peers", msg_content, // List of known peers
		)

	for _, conn := range n.activeCon {
		n.SendMessage(responseMsg, &conn)
	}
	format.Display_network(n.GetName(), "acceptAdmission()", "Sent admission response to all active connections.")

	// Maybe I am the node connected to the new node! If this is the case, I need to accept the new node
	n.handleAdmissionGranted(responseMsg, conn) // Call handleAdmissionGranted to process the admission granted message
}


// handleAdmissionGranted processes an admission granted message
// Two cases:
// 1. For the requesting node, it means it has been admitted and can start its activity.
// 2. For the node connected to the requesting node, it means it can now accept the new node
func (n* NetworkLayer) handleAdmissionGranted(msg string, conn net.Conn) {

	n.mu.Lock()
	waitingConn := n.waitingConn
	waitingForAdmission := n.waitingForAdmission
	requestedConnections := format.Findval(msg, "requested_connections")
	activeCon := n.activeCon // Get the current active connections
	n.mu.Unlock()

	if waitingForAdmission {
		n.mu.Lock()
		n.waitingForAdmission = false 		// Admitted!
		n.waitingConn = nil 			// Reset waiting connection after admission granted
		n.activeCon = append(n.activeCon, conn) // Add the new connection to active connections
		format.Display_network(n.GetName(), "handleAdmissionGranted()", "Admission granted! I am now connected to: " + conn.RemoteAddr().String())

		// Update list of known peers 
		var names_in_message string = format.Findval(msg, "known_peers")
		sites_parts := strings.Split(names_in_message, utils.PearD_SITE_SEPARATOR)
		n.knownPeers = []string{} // Reset known peers
		for _, site := range sites_parts {
			n.knownPeers = append(n.knownPeers, site)
		}
		n.mu.Unlock()
	} else if waitingConn != nil { // I am one of the node the new node is connected to

		// Send admission granted to the waiting new node
		if format.Findval(msg, "requester") == n.nodeRequestingElection {
			granted_msg := format.Replaceval(msg, "sender_name", n.GetName())
			n.SendMessage(granted_msg, &waitingConn)
			format.Display_network(n.GetName(), "handleAdmissionGranted()", "Sent admission granted to waiting connection: " + waitingConn.LocalAddr().String())
			n.mu.Lock()
			n.activeCon = append(n.activeCon, waitingConn) // Add the new connection to active connections
			n.waitingConn = nil // Reset waiting connection after sending the admission granted message
			n.mu.Unlock()
		} else {
			format.Display_w(n.GetName(), "handleAdmissionGranted()", "Waiting connection name ("+ n.nodeRequestingElection+") does not match requester name in message. Ignoring admission granted.")
		}

		// I sended the admission granted message to the waiting connection,
		// so now I need to ask the peer I am connected to and to which the new node is connected to,
		// close our connection. This will thus maintain the ring network.
		msg_close := format.Build_msg(
			"sender_name", n.GetName(),
			"type", "close_connection",
			"id", n.GenerateUniqueMessageID(),
			"destination", "network")
		for _, c := range activeCon {
			trimmedAddr := strings.TrimPrefix(c.LocalAddr().String(), "127.0.0.1:")
			if strings.Contains(requestedConnections, trimmedAddr) {
				n.SendMessage(msg_close, &c) // Send close connection message to the peer I am connected to
			} else {
				format.Display_network(n.GetName(), "handleAdmissionGranted()", "Not sending close connection message to "+trimmedAddr+" as it is not in the requested connections: "+requestedConnections)
			}
		}

		// Add this node to known peers
		n.mu.Lock()
		n.knownPeers = append(n.knownPeers, format.Findval(msg, "requester")) // Add the requester to known peers
		n.mu.Unlock()
	} else {
		format.Display_network(n.GetName(), "handleAdmissionGranted()", "Reset election parameters as no waiting connection found.")
		n.knownPeers = append(n.knownPeers, format.Findval(msg, "requester")) // Add the requester to known peers
	}

	n.mu.Lock() // Reset election parameters
	n.electedLeader = ""
	n.electionParent = nil
	n.expectedAnswers = -1 // Reset expected answers 
	n.nodeRequestingElection = "" // Reset the node requesting the election
	n.networkIsRing = true

	format.Display_network(n.GetName(), "handleAdmissionGranted()", "Known peers: " + strings.Join(n.knownPeers, ", "))
	n.mu.Unlock()
}

// ============================ ID WATCHER FUNCTIONS ============================
// Return true if the message's ID is contained with
// one of the network's ID pairs. If it is not contains,
// it adds it to the interval (see utils.message_watch)
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
	n.mu.Lock()
	if n.IDWatcher.ContainsMID(sender_name, msg_NbMessageSent) == false {
		n.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
		n.mu.Unlock()
		return false
	}
	n.mu.Unlock()
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

	n.mu.Lock()
	n.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
	n.mu.Unlock()
}
