package node

import (
	"strings"
	"sync"
	"distributed_system/format"
	"distributed_system/models"
	"distributed_system/utils"
	"log" // Added for logging errors
	"strconv"
)

// UserNode represents a user in the system
type UserNode struct {
	BaseNode
	predictionFunc	   func (values []float32, decay float32) float32
	decayFactor	   float32 		       // Decay factor in some prediction functions
	lastPrediction     *float32                    // pointer to allow nil value at start
	recentReadings     map[string][]models.Reading // FIFO queue per sender
	recentPredictions  map[string][]float32      	// FIFO queue per sender of made predictions
	verifiedItemIDs    map[string][]string         // Tracks the verified item for each sender by their ID
	mutex		   sync.Mutex
}

// NewUserNode creates a new user node
func NewUserNode(id string, model string) *UserNode {

	// Set the prediction function based on the model type
	var predFunction func (values []float32, decay float32) float32
	var decayFactor float32 = 0.0
	if model == "exp" {
		predFunction = models.DecayedWeightedMean
		decayFactor = utils.DECAY_FACTOR
	} else {
		predFunction = models.LinearMean
		decayFactor = 0.0
	}

	return &UserNode{
		BaseNode:           NewBaseNode(id, "user"),
		predictionFunc:     predFunction,
		decayFactor:        decayFactor,
		lastPrediction:     nil,
		recentReadings:     make(map[string][]models.Reading),
		recentPredictions:  make(map[string][]float32),
		verifiedItemIDs:    make(map[string][]string),
		mutex: 		    sync.Mutex{},
	}
}

// Start begins the verifier's operation
func (u *UserNode) Start() error {
	format.Display(format.Format_d("node_user.go", "Start()", "Starting user node "+u.GetName()))
	u.isRunning = true
	return nil
}

// HandleMessage processes incoming messages from control layer
func (u *UserNode) HandleMessage(channel chan string) {

	for msg := range channel {
		var msg_clock string = format.Findval(msg, "clk", u.GetName())

		// Update Lamport clock based on received message
		msg_clock_int, _ := strconv.Atoi(msg_clock)

		u.mutex.Lock()
		u.clock = utils.Synchronise(u.clock, msg_clock_int)
		u.mutex.Unlock()

		var msg_type string = format.Findval(msg, "type", u.GetName())
		var msg_content_value string = format.Findval(msg, "content_value", u.GetName())
		var msg_sender string = format.Findval(msg, "sender_name", u.GetName())

		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store for the specific sender
			format.Display(format.Format_d(
				"node_user.go", "HandleMessage()",
				u.GetName()+" received the new reading <"+msg_content_value+"> from "+msg_sender))

			// Parse the reading value
			readingVal, err := strconv.ParseFloat(msg_content_value, 32)

			if err != nil {
				log.Printf("%s: Error parsing reading value '%s': %v", u.GetName(), msg_content_value, err)
				continue // Skip this message if parsing fails
			}

			// Get or create the queue for the sender
			u.mutex.Lock()
			queue, exists := u.recentReadings[msg_sender]
			if !exists {
				queue = make([]models.Reading, 0, utils.VALUES_TO_STORE)
			}

			// Add to FIFO queue for this sender
			if len(queue) >= utils.VALUES_TO_STORE {

				// Remove all occurrences of the readingID from the verifiedItemIDs map
				for senderID, itemIDs := range u.verifiedItemIDs {
					u.verifiedItemIDs[senderID] = utils.RemoveAllOccurrencesString(itemIDs, queue[0].ReadingID)
					}

				// Remove the oldest element (slice trick)
				queue = queue[1:]
			}
			queue = append(queue, models.Reading{
				ReadingID: format.Findval(msg, "item_id", u.GetName()),
				Temperature: float32(readingVal),
				Clock:   msg_clock_int,
				SensorID:    msg_sender,
				IsVerified:  false,
			})
			u.recentReadings[msg_sender] = queue // Update the map
			u.mutex.Unlock()

			u.processDatabse()
		case "lock_release_and_verified_value":
			go u.handleLockRelease(msg)
		}

	}
}

// handleLockRelease processes a lock release message from a verifier
// which also contains the verified value.
func (n *UserNode) handleLockRelease(msg string) {
	// Extract information from the message
	itemID := format.Findval(msg, "item_id", n.GetName())
	sensorID := strings.Split(itemID, "_")[0]
	verifier := format.Findval(msg, "sender_name_source", n.GetName())
	verifiedValueStr := format.Findval(msg, "content_value", n.GetName())
	verifiedValue, err := strconv.ParseFloat(verifiedValueStr, 32)
	if err != nil {
		format.Display(format.Format_e(n.GetName(), "handleLockRelease()", "Error parsing verified value: "+verifiedValueStr))
		return
	}

	n.mutex.Lock()

	// By the time the verification is done, the item might have been gone (erased 
	// because new readings were received). If it is the case (ie. the itemID don't 
	// exists anymore), no need to update the verifiedItemIDs nor recentReadings:
	isItemInReadings := false 
	readingIndex := -1
	for i, reading := range n.recentReadings[sensorID] {
		if reading.ReadingID == itemID {
			isItemInReadings = true 
			readingIndex = i
			break
		}
	}
	if isItemInReadings == false {
		n.mutex.Unlock()
		return
	}

	// Update the verified item list for this sender
	if _, exists := n.verifiedItemIDs[sensorID]; exists {
		n.verifiedItemIDs[sensorID] = append(n.verifiedItemIDs[sensorID], itemID)
	} else {
		n.verifiedItemIDs[sensorID] = make([]string, 0)
		n.verifiedItemIDs[sensorID] = append(n.verifiedItemIDs[sensorID], itemID)
	}
	n.mutex.Unlock()

	// Update verified value:
	n.mutex.Lock()
	n.recentReadings[sensorID][readingIndex].IsVerified = true
	n.recentReadings[sensorID][readingIndex].Temperature = float32(verifiedValue)
	n.recentReadings[sensorID][readingIndex].VerifierID = verifier
	n.mutex.Unlock()
	n.processDatabse()
}


func (n *UserNode) processDatabse() {

	// One prediction per sensor: 
	n.mutex.Lock()
	recentReadings := n.recentReadings 
	n.mutex.Unlock()
	for sensor, readings := range recentReadings {
		var readingValues []float32 = make([]float32, len(readings))
		for _, r := range readings {
			readingValues = append(readingValues, r.Temperature)
		}
		prediction := n.predictionFunc(readingValues, n.decayFactor)
		// Get or create the queue for the sender

		n.mutex.Lock()
		queue, exists := n.recentPredictions[sensor]
		if !exists {
			queue = make([]float32, 0, utils.VALUES_TO_STORE)
		}

		// Add to FIFO queue for this sender
		if len(queue) >= utils.VALUES_TO_STORE {
			// Remove the oldest element (slice trick)
			queue = queue[1:]
		}
		queue = append(queue, prediction)
		n.recentPredictions[sensor] = queue // Update the map
		n.mutex.Unlock()
	}
	n.printDatabase()
}

func (n *UserNode) printDatabase() {
	var debug string = n.GetName()+" database:\n"
	n.mutex.Lock()
	for sensor, readings := range n.recentReadings {
		debug += sensor + "\n"
		for _, r := range readings {
			debug += "	" + r.ReadingID + " status:" + strconv.FormatBool(r.IsVerified) + " verifier:" + r.VerifierID + "\n"
		}
	} 
	
	// Predictions 
	for sensor, predictions := range n.recentPredictions {
		prediction := predictions[len(predictions)-1] // Get the last prediction
		debug += "Latest prediction for "+sensor+": " + strconv.FormatFloat(float64(prediction), 'f', -1, 32) + "\n"
	}


	n.mutex.Unlock()
	format.Display(format.Format_e(n.GetName(), "handleLockRelease()", debug))

}
