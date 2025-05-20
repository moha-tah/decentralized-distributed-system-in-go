package node

import (
	"bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"distributed_system/format"
	"distributed_system/utils"
	"encoding/csv"
	"fmt"
	"log"
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
}

type SnapshotData struct {
	VectorClock []int
	Values      []float32
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
    }
}

// Start begins the control operations
func (c *ControlLayer) Start() error {
	format.Display(format.Format_d("Start()", "control_layer.go", "Starting control layer "+c.GetName()))

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
			time.Sleep(time.Duration(1) * time.Second)
			c.SendPearDiscovery()
			format.Display(format.Format_d(c.GetName(), "SendMsg()", "Sending pear discovery message..."))
			time.Sleep(time.Duration(1) * time.Second)
			format.Display(format.Format_d(c.GetName(), "SendMsg()", "Waiting for pear discovery answers..."))
			c.ClosePearDiscovery() // Will send known names to all nodes

			// Start the child only after
			c.child.Start()
		}()
		//demande de snapshot après 5 secondes
		go func() {
			time.Sleep(5 * time.Second) // attendre que tout soit initialisé
			format.Display(format.Format_d("Start()", c.GetName(), "⏳ 20s écoulées, envoi de la requête snapshot..."))
			c.RequestSnapshot()
			format.Display(format.Format_d("Start()", c.GetName(), "📤 snapshot_request envoyé depuis "+c.GetName()))
		}()
	} else {
		go func() { // Wake up child only after pear discovery is finished
			time.Sleep(time.Duration(2) * time.Second)

			// Notify the child that its control layer has been created
			c.child.Start()
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
	if c.SawThatMessageBefore(msg) {
		return nil
	} 

	if !strings.Contains(msg, "pear_discovery") {
		// Only print to stderr msg that are not related to pear_discovery
		// (overwhelming)
		// format.Display(format.Format_w(c.GetName(), "HandleMsg()", "Received: "+msg))
	}

	c.mu.Lock()
	// Update vector clock 
	if c.vectorClockReady {
		recVC := format.RetrieveVectorClock(msg, len(c.vectorClock), c.vectorClockReady)
		c.vectorClock = utils.SynchroniseVectorClock(c.vectorClock, recVC, c.nodeIndex)
	} else {
		resClk, _ := strconv.Atoi(format.Findval(msg, "clk", c.GetName()))
		c.clk = utils.Synchronise(c.clk, resClk)
	}
	c.mu.Unlock()

	// Extract msg caracteristics
	var msg_destination string = format.Findval(msg, "destination", c.GetName())
	var msg_type string = format.Findval(msg, "type", c.GetName())
	// var msg_content_value string = format.Findval(msg, "content_value", c.GetName())
	// Original Sender: (used in IDWatcher to check if a msg from this node has already
	// been received)
	var sender_name_source string = format.Findval(msg, "sender_name_source", c.GetName())

	// Will be used at the end to check if
	// control layer needs to resend the message to all other nodes
	var propagate_msg bool = false

	if msg_destination == "applications" {
		switch msg_type {
		case "new_reading":
			// Send to child app
			c.SendMsg(msg, true)

			propagate_msg = true

		case "snapshot_request":
			// c.channel_to_application <- msg
			c.SendMsg(msg, true)
			format.Display(format.Format_d(
				c.GetName(), "HandleMessage()",
				"📥 snapshot_request reçu depuis "+sender_name_source))
			propagate_msg = true
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
				// 💡The node responsible of pear discovery will also do the following, as
				// no condition on node id. Done that way in order to have the message dispayling
				// the number of known nodes, even for the node 0.
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


				// ✅ INITIALISATION DU VECTEUR DANS L’ENFANT
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
				//
				// format.Display(format.Format_e(
				// 	c.GetName(), "HandleMsg()",
				// 	"Updated nb sites to "+strconv.Itoa(c.nbOfKnownSites)))
			}

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
			// message propagation (see case below) (which is a mandatory feature
			// -to bring message all the way to node 0- not a problem)
			if !slices.Contains(c.knownSiteNames, siteName) {
				c.knownSiteNames = append(c.knownSiteNames, siteName)
				c.nbOfKnownSites += 1
			} 
			if verifierName != "" && !slices.Contains(c.knownVerifierNames, verifierName) {
				c.knownVerifierNames = append(c.knownVerifierNames, verifierName)
			}

		case "snapshot_response":

			if c.id != "0_control" {
				return nil
			}

			format.Display(format.Format_d(c.GetName(), "HandleMessage()", "📥 snapshot_response reçu from "+sender_name_source))

			// Mise à jour de l'horloge vectorielle locale
			vcStr := format.Findval(msg, "vector_clock", c.GetName())
			recvVC, err := utils.DeserializeVectorClock(vcStr)
			if err != nil {
				format.Display(format.Format_e(c.GetName(), "HandleMessage()", "Erreur parsing vector_clock: "+err.Error()))
				return nil
			}
			c.mu.Lock()
			for i := 0; i < len(c.vectorClock); i++ {
				c.vectorClock[i] = utils.Max(c.vectorClock[i], recvVC[i])
			}
			c.vectorClock[c.nodeIndex]++
			c.mu.Unlock()

			// Extraction des données du snapshot
			sensorID := format.Findval(msg, "sender_name_source", c.GetName())
			valuesStr := format.Findval(msg, "content_value", c.GetName())
			format.Display(format.Format_d(c.GetName(), "HandleMessage()", "Received snapshot from "+sensorID+" with values: "+valuesStr))
			values := utils.ParseFloatArray(valuesStr)

			// Enregistrement du snapshot
			c.mu.Lock()
			c.receivedSnapshots[sensorID] = SnapshotData{
				VectorClock: recvVC,
				Values:      values,
			}
			c.mu.Unlock()

			// Vérifie si tous les capteurs ont répondu
			// sensorNames := c.getSensorNames() // Fonction à ajouter : filtre les knownSiteNames pour ne garder que ceux qui commencent par "sensor"
			if len(c.receivedSnapshots) == len(c.knownSiteNames)-1 {
				go c.CheckSnapshotCoherence() // méthode que tu dois avoir déjà codée
			} else {
				format.Display(format.Format_d(c.GetName(), "HandleMessage()", "Stil missing "+strconv.Itoa(len(c.knownSiteNames)-1-len(c.receivedSnapshots))+" sensors"))
format.Display(format.Format_d(c.GetName(), "HandleMessage()", "Received snapshots: "+strconv.Itoa(len(c.receivedSnapshots))))
			}
		}

	} else if msg_destination == "verifiers" && c.child.Type() == "verifier" {
		// The msg is for the child app layer (which is a verifier)
		// Send to child app through channel 
		c.SendMsg(msg, true) // through channel
		// Propagate to other verifiers
		propagate_msg = true
	} else if msg_destination == c.child.GetName() {
		// The msg is directly for the child app layer
		// Send to child app through channel
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
		}
	}


	// Propagate the message to other nodes if needed
	if propagate_msg { 
		// As it is a propagation, use the same msg_id as in received msg 
		// so no id modification.
		propagate_msg := format.Replaceval(msg, "sender_name", c.GetName())
		propagate_msg = format.Replaceval(msg, "sender_type", "control")
		c.SendMsg(propagate_msg)
	}

	return nil
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
	recVC := format.RetrieveVectorClock(msg, len(c.vectorClock), c.vectorClockReady)
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
	} else {
		c.clk += 1 // Incrément de l'horloge locale
		msg = format.Replaceval(msg, "clk", strconv.Itoa(c.clk))
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

func (c *ControlLayer) RequestSnapshot() {
	msgID := c.GenerateUniqueMessageID()
	msg := format.Msg_format_multi(format.Build_msg_args(
		"id", msgID,
		"type", "snapshot_request",
		"sender_name", c.GetName(),
		"sender_name_source", c.GetName(),
		"sender_type", "control",
		"destination", "applications", // pour que le msg soit traité par tous les capteurs via control layers
		"clk", "", // Is done in c.SendMsg
		"vector_clock", "", // Is done in c.SendMsg
		"content_type", "request_type",
		"content_value", "snapshot_start",
	))

	c.SendMsg(msg)
	c.SendMsg(msg, true)
}



func (c *ControlLayer) CheckSnapshotCoherence() {
	c.mu.Lock()

	// Copie des données locales pour traitement sans verrou
	snapshots := make(map[string]SnapshotData)
	for id, snap := range c.receivedSnapshots {
		if !strings.Contains(id, "control") {
			snapshots[id] = snap
		}
	}
	c.mu.Unlock()

	ids := []string{}
	for id := range snapshots {
		ids = append(ids, id)
	}

	// Cas trivial
	if len(ids) <= 1 {
		c.SaveSnapshotToCSVThreadSafe(snapshots)
		return
	}

	// Vérifier compatibilité
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if !utils.VectorClockCompatible(snapshots[ids[i]].VectorClock, snapshots[ids[j]].VectorClock) {
				c.SaveSnapshotToCSVThreadSafe(snapshots)
				return
			}
		}
	}

	c.SaveSnapshotToCSVThreadSafe(snapshots)
}

func (c *ControlLayer) getSensorNames() []string {
	sensors := []string{}
	for _, name := range c.knownSiteNames {
		if strings.HasPrefix(name, "sensor") {
			sensors = append(sensors, name)
		}
	}
	return sensors
}

func (c *ControlLayer) PrintSnapshotsTable() {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Println()
	fmt.Println("📦  Snapshots reçus (" + strconv.Itoa(len(c.receivedSnapshots)) + " capteurs)")
	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Printf("| %-15s | %-22s | %-15s |\n", "Capteur", "Horloge vectorielle", "Valeurs")
	fmt.Println("────────────────────────────────────────────────────────────")

	for sensor, snapshot := range c.receivedSnapshots {
		vcStr := fmt.Sprintf("%v", snapshot.VectorClock)
		valStr := fmt.Sprintf("%v", snapshot.Values)
		if len(valStr) > 15 {
			valStr = valStr[:12] + "..."
		}
		fmt.Printf("| %-15s | %-22s | %-15s |\n", sensor, vcStr, valStr)
	}

	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Println()
}

func (c *ControlLayer) SaveSnapshotToCSV() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Nom dynamique basé sur l’horloge Lamport du contrôleur
	filename := fmt.Sprintf("snapshot_%d.csv", c.vectorClock[c.nodeIndex])

	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("❌ Erreur création fichier %s: %v\n", filename, err)
		return
	}
	defer file.Close()

	// Écrire l’en-tête
	_, _ = file.WriteString("Capteur,HorlogeVectorielle,Valeurs\n")

	// Écrire chaque ligne
	for sensor, snapshot := range c.receivedSnapshots {
		vcStr := strings.ReplaceAll(fmt.Sprint(snapshot.VectorClock), " ", "")
		valStr := strings.ReplaceAll(fmt.Sprint(snapshot.Values), " ", "")
		line := fmt.Sprintf("%s,\"%s\",\"%s\"\n", sensor, vcStr, valStr)
		_, _ = file.WriteString(line)
	}

	fmt.Printf("📁 État global sauvegardé dans %s\n", filename)
}

func (c *ControlLayer) SaveSnapshotToCSVThreadSafe(snapshots map[string]SnapshotData) {
	file, err := os.Create("snapshot.csv")
	if err != nil {
		log.Println("Erreur création fichier snapshot:", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"ID", "Vector Clock", "Values"})

	for id, snap := range snapshots {
		writer.Write([]string{
			id,
			utils.SerializeVectorClock(snap.VectorClock),
			strings.Join(utils.Float32SliceToStringSlice(snap.Values), ";"),
		})
	}
}
