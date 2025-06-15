package node

import (
	"distributed_system/models"
	"distributed_system/utils"
	"strconv"
	"sync"
)

// Node defines the interface for all node types in the system
type Node interface {
	// Start begins the node's operation with the given context
	Start() error

	// Stop gracefully terminates the node's operation
	// Stop() error

	// ID returns the unique identifier for this node
	ID() string

	// Type returns the type of this node: "sensor", "verifier", or "user"
	Type() string

	// HandleMessage processes an incoming message from another node
	HandleMessage(chan string)

	// Gets name and id in a single string
	GetName() string

	// SetControlLayer is used to transmit information to other nodes
	SetControlLayer(*ControlLayer) error

	// InitVectorClockWithSites initializes the vector clock with the given site names
	InitVectorClockWithSites(siteNames []string)

	// GetVectorClock returns a copy of the node's vector clock.
	GetVectorClock() []int

	// SetVectorClock updates the node's vector clock and related properties.
	SetVectorClock(newVC []int, siteNames []string)

	GetLocalState() string

	// GetApplicationState returns the application-specific state of the node.
	GetApplicationState() map[string][]models.Reading

	// SetApplicationState sets the application-specific state of the node.
	SetApplicationState(state map[string][]models.Reading)

	// Used only for Users, from control layer, to update web UI 
	SetSnapshotInProgress(inProgress bool)
}

// BaseNode implements common functionality for all node types
type BaseNode struct {
	id               string
	mu               sync.Mutex
	nodeType         string
	isRunning        bool
	ctrlLayer        *ControlLayer
	nbMsgSent        int
	clk              int         // Temporary variable for the vector clock
	vectorClock      []int       // taille = nombre total de noeuds
	vectorClockReady bool        // true après pear_discovery_sealing
	nodeIndex        int         // position de ce node dans le vecteur
	channel_to_ctrl  chan string // channel to send messages to the control layer
}

// NewBaseNode creates a new base node with the given ID and type
func NewBaseNode(id, nodeType string) BaseNode {
	return BaseNode{
		id:               id,
		mu:               sync.Mutex{},
		nodeType:         nodeType,
		isRunning:        false,
		nbMsgSent:        0,
		clk:              0,
		nodeIndex:        0,
		vectorClockReady: false,
		channel_to_ctrl:  make(chan string, 10), // Buffered channel for control messages
	}
}

// ID returns the node's unique identifier
func (n *BaseNode) ID() string {
	return n.id
}

// Type returns the node's type
func (n *BaseNode) Type() string {
	return n.nodeType
}

func (n *BaseNode) NbMsgSent() int {
	return n.nbMsgSent
}

func (n *BaseNode) GetName() string {
	return n.nodeType + " (" + n.id + ")"
}

// func (n *BaseNode) GetControlName() string {
// 	return "control (" + n.id + "_control)"
// }

func (n *BaseNode) SetControlLayer(c *ControlLayer) error {
	n.mu.Lock()
	n.ctrlLayer = c
	n.mu.Unlock()
	go n.ctrlLayer.HandleMessage(n.channel_to_ctrl)
	return nil
}

func (n *BaseNode) GetVectorClock() []int {
	n.mu.Lock()
	defer n.mu.Unlock()
	// Return a copy to prevent race conditions
	vcCopy := make([]int, len(n.vectorClock))
	copy(vcCopy, n.vectorClock)
	return vcCopy
}

func (n *BaseNode) SetVectorClock(newVC []int, siteNames []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.vectorClock = newVC
	// Use the control layer's name to find the index, as this is what is in siteNames
	n.nodeIndex = utils.FindIndex(n.ctrlLayer.GetName(), siteNames)
	n.vectorClockReady = true
}

func (n *BaseNode) GenerateUniqueMessageID() string {
	return n.Type() + "_" + n.ID() + "_" + strconv.Itoa(n.NbMsgSent())
}

func (n *BaseNode) GetApplicationState() map[string][]models.Reading {
	// Base implementation, should be overridden by child nodes.
	return make(map[string][]models.Reading)
}

func (n *BaseNode) SetApplicationState(state map[string][]models.Reading) {
	// Base implementation, should be overridden by child nodes.
}

func (n *BaseNode) InitVectorClockWithSites(siteNames []string) {
	n.mu.Lock()
	n.vectorClock = make([]int, len(siteNames))
	for i, name := range siteNames {
		if name == n.GetName() {
			n.nodeIndex = i
			break
		}
	}
	n.mu.Unlock()
}
