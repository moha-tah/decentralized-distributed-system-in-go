package node

import (
	"strconv"
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

}

// BaseNode implements common functionality for all node types
type BaseNode struct {
	id        	string
	nodeType  	string
	isRunning 	bool
	clock     	int	
	ctrlLayer 	*ControlLayer
	nbMsgSent	int
}

// NewBaseNode creates a new base node with the given ID and type
func NewBaseNode(id, nodeType string) BaseNode {
	return BaseNode{
		id:        id,
		nodeType:  nodeType,
		isRunning: false,
		clock:     0,
		nbMsgSent: 0,
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

func (n *BaseNode) SetControlLayer(c *ControlLayer) error {
	n.ctrlLayer = c 
	return nil
}

func (n* BaseNode) GenerateUniqueMessageID() string {
	return n.Type() + "_" + n.ID() + "_" + strconv.Itoa(n.NbMsgSent())
}
