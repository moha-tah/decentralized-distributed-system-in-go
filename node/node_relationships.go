package node

import (
	"distributed_system/format"
	"distributed_system/utils"
	"slices"
	"strconv"
	"strings"
)

// Tree construction, Pear discovery, Neighbor discovery, ...

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
		"clk", "", // changed in SendMsg
		"vector_clock", "", // changed in SendMsg
		"child", "false",
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

func (c* ControlLayer) processRedTree(msg string) {
	sender_name_source := format.Findval(msg, "sender_name_source", c.GetName())
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
				"clk", "", // changed in SendMsg
				"vector_clock", "", // changed in SendMsg
				"child", "true", // Notify parent that I am a child
			))
			c.SendMsg(tree_answer)
		}
	}
}


func (c *ControlLayer) SendPearDiscoveryAnswer(msg string) {
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

	slices.Sort(c.knownSiteNames)
	msg_content := strings.Join(c.knownSiteNames, utils.PearD_SITE_SEPARATOR)
	if len(c.knownVerifierNames) > 0 {
		msg_content += utils.PearD_VERIFIER_SEPARATOR + strings.Join(c.knownVerifierNames, utils.PearD_SITE_SEPARATOR)
	}

	// Display update nb of site as all other nodes will do when receiving:
	format.Display(format.Format_g(c.GetName(), "ClosePearDis()", "Updated nb sites to "+strconv.Itoa(c.nbOfKnownSites)))

	knownSiteNames := c.knownSiteNames
	c.mu.Unlock()

	// Send final sealing message with control names + verifier names
	c.SendControlMsg(msg_content, "siteNames", "pear_discovery_sealing", "control", "", c.GetName())

	// ✅ Init vector clocks of current control layer & child's
	c.InitVectorClockWithSites(knownSiteNames)
	c.child.InitVectorClockWithSites(knownSiteNames)
}

func (c* ControlLayer) HandlePearDiscoverySealing(msg string) {

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

func (c* ControlLayer) HandlePearDiscoveryAnswerFromResponsibleNode(msg string) {
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
	c.mu.Lock()
	if !slices.Contains(c.knownSiteNames, siteName) {
		c.knownSiteNames = append(c.knownSiteNames, siteName)
		c.nbOfKnownSites += 1
	}
	if verifierName != "" && !slices.Contains(c.knownVerifierNames, verifierName) {
		c.knownVerifierNames = append(c.knownVerifierNames, verifierName)
	}
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
