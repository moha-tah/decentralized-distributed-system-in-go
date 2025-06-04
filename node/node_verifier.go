package node

import (
	"strconv"
	"time"
	"os"
	"distributed_system/format"
	"distributed_system/utils"
	"distributed_system/models"
	"fmt"
	"strings"
	"slices" // for `slices.Contains` function
)

// VerifierNode represents a verifier in the system
type VerifierNode struct {
	BaseNode
	processingCapacity int
	threshold          float32
	verificationLocks  map[string]bool            // Maps item IDs to lock status
	lockRequests       map[string]map[string]int// Maps item IDs to node IDs and timestamps
	lockedItems        map[string]string          // Maps item IDs to noded locking each one
	lockReplies        map[string]map[string]bool // Maps item IDs to node IDs and reply status
	otherVerifiers     map[string]bool            // Set of other verifier node IDs
	nbOfVerifiers      int			      // Number of OTHER verifiers in the network
	recentReadings     map[string][]models.Reading// FIFO queue per sender
	processingItems    map[string]bool            // Items currently being processed by this verifier
	pendingItems       []models.Reading           // Queue of items waiting to be processed
	verifiedItemIDs    map[string][]string        // Tracks the verified item for each sender by their ID
	baseTemp     []float32 			      // Base temperature [low, high]
}

// NewVerifierNode creates a new verifier node
func NewVerifierNode(id string, capacity int, threshold float32, baseTempLow float32, baseTempHigh float32) *VerifierNode {
	return &VerifierNode{
		BaseNode: NewBaseNode(id, "verifier"),
		processingCapacity: capacity,
		threshold:          threshold,
		verificationLocks:  make(map[string]bool),
		lockRequests:       make(map[string]map[string]int),
		lockReplies:        make(map[string]map[string]bool),
		lockedItems:        make(map[string]string),
		otherVerifiers:     make(map[string]bool),
		nbOfVerifiers:      0,
		recentReadings:     make(map[string][]models.Reading),
		processingItems:    make(map[string]bool),
		pendingItems:       make([]models.Reading, 0),
		verifiedItemIDs:    make(map[string][]string),
		baseTemp:     	    []float32{baseTempLow, baseTempHigh},
	}
}

// Start begins the verifier's operation
func (v *VerifierNode) Start() error {
	format.Display(format.Format_d("Start()", "node_verifier.go", "Starting verifier node "+v.GetName()))

	v.isRunning = true
	
	// Start a goroutine to periodically check for unverified items
	go func() {
		ticker := time.NewTicker(1 * time.Second)
				
		v.mu.Lock()
		isRunning := v.isRunning 
		processingCapacity := v.processingCapacity
		v.mu.Unlock()
		
		for isRunning {
			select {
			case <-ticker.C:
				v.mu.Lock()
				isRunning = v.isRunning 
				l_processing := len(v.processingItems)
				v.mu.Unlock()

				// Only check for unverified items if we're not already processing 
				// more than the processing capacity.
				if l_processing < processingCapacity { // Search for new only if not full already
					go v.CheckUnverifiedItems()
				} else {
					// // debug:
					// processingItemID := ""
					// for itemID := range v.processingItems {
					// 	processingItemID = itemID
					// 	break
					// }
					// format.Display(format.Format_d(v.GetName(), "Start()", "Already processing "+ processingItemID))
				}
			}
		}
		ticker.Stop()
	}()

	return nil
}

func (v *VerifierNode) InitVectorClockWithSites(sites []string) {
	v.mu.Lock()
	v.vectorClock = make([]int, len(sites))
	v.nodeIndex = utils.FindIndex(v.ctrlLayer.GetName(), sites)
	v.vectorClockReady = true
	v.mu.Unlock()
}

// HandleMessage processes incoming messages from control app
func (v *VerifierNode) HandleMessage(channel chan string) {
	    defer func() {
		if r := recover(); r != nil {
			format.Display(format.Format_w(v.GetName(), "HandleMessage()", "Recovered from panic: "+fmt.Sprint(r)))
		    // Optionally restart the message handling
		    go v.HandleMessage(channel)
		}
	    }()

	for msg := range channel {
		v.mu.Lock()

		rec_clk, _ := strconv.Atoi(format.Findval(msg, "clk"))
		v.clk = utils.Synchronise(v.clk, rec_clk)
		clk_int := v.clk
		// if v.vectorClockReady == true {
			recVC := format.RetrieveVectorClock(msg, len(v.vectorClock))
			v.vectorClock = utils.SynchroniseVectorClock(v.vectorClock, recVC, v.nodeIndex)
		// }
		v.mu.Unlock()

		var msg_type string = format.Findval(msg, "type")
		var msg_content_value string = format.Findval(msg, "content_value")
		var msg_sender string = format.Findval(msg, "sender_name_source") // Get sender ID

		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store for the specific sender

			// Parse the reading value
			readingVal, err := strconv.ParseFloat(msg_content_value, 32)
			if err != nil {
				format.Display(format.Format_e(v.GetName(), "HandleMessage","Error parsing reading value: "+msg_content_value))
				continue // Skip this message if parsing fails
			}

			format.Display(format.Format_d(
				"node_user.go", "HandleMessage()",
				v.GetName()+" received the new reading <"+msg_content_value+"> from "+msg_sender))
			// Get or create the queue for the sender
			v.mu.Lock()
			queue, exists := v.recentReadings[msg_sender]
			if !exists {
				queue = make([]models.Reading, 0, utils.VALUES_TO_STORE)
			}

			// Add to FIFO queue for this sender
			if len(queue) >= utils.VALUES_TO_STORE {
				
				v.removeAllExistenceOfReading(queue[0].ReadingID)
			
				// Remove the oldest element (slice trick)
				queue = queue[1:]
			}
			queue = append(queue, models.Reading{
				ReadingID: format.Findval(msg, "item_id"),
				Temperature: float32(readingVal),
				Clock:      clk_int,
				SensorID:    msg_sender,
				IsVerified:  false,
			})
			
			v.recentReadings[msg_sender] = queue // Update the map

			// Définir le code d'état
			var stateCode int
			if float32(readingVal) > v.threshold || float32(readingVal) < -v.threshold {
				stateCode = 2 // invalid
			} else {
				stateCode = 1 // valid
			}

			v.mu.Unlock()

			// Sauvegarde dans le fichier log
			v.logReceivedReading(msg_sender, readingVal, stateCode)


		
		case "snapshot_request":
			format.Display(format.Format_d(
				v.GetName(), "HandleMessage()",
				"📦 snapshot_request reçu"))

			vcStr := format.Findval(msg, "vector_clock")
			recvVC, err := utils.DeserializeVectorClock(vcStr)
			if err == nil {
				for i := 0; i < len(v.vectorClock); i++ {
					v.vectorClock[i] = utils.Max(v.vectorClock[i], recvVC[i])
				}
				v.vectorClock[v.nodeIndex] += 1
			}

			// Créer la réponse avec l'horloge vectorielle uniquement
			msgID := v.GenerateUniqueMessageID()
			response := format.Msg_format_multi(format.Build_msg_args(
				"id", msgID,
				"type", "snapshot_response",
				"sender_name", v.GetName(),
				"sender_name_source", v.GetName(),
				"sender_type", v.Type(),
				"destination", format.Findval(msg, "sender_name_source"),
				"clk", strconv.Itoa(v.clk),
				"vector_clock", utils.SerializeVectorClock(v.vectorClock),
				"content_type", "snapshot_data",
				"content_value", "[]", // les verifieurs ne stockent pas de valeurs
			))

			format.Display(format.Format_d(v.GetName(), "HandleMessage()", "Sending snapshot_response"))
			if v.ctrlLayer.id != "0_control" {
				// v.ctrlLayer.SendApplicationMsg(response)
				v.SendMessage(response)
			} else {
				// v.ctrlLayer.HandleMessage(response)
				v.SendMessage(response, true)
			}
			
		case "lock_request":
			go v.handleLockRequest(msg)
			
		case "lock_reply":
			go v.handleLockReply(msg)
			
		case "lock_release_and_verified_value":
			go v.handleLockRelease(msg)
	
		case "lock_acquired":
			go v.handleLockAcquired(msg)

		case "lock_request_cancelled":
			// Remove the lock request for this item 
			itemID := format.Findval(msg, "item_id") 
			v.mu.Lock() 
			// Remove the lock request from our map 
			delete(v.lockRequests, itemID) 
			// Remove the lock replies for this item 
			delete(v.lockReplies, itemID) 
			v.mu.Unlock()

		case "pear_discovery_verifier":
			// This message is recieved from our control layer and 
			// contains the list of verifiers in the network.
			site_names := strings.Split(msg_content_value, utils.PearD_SITE_SEPARATOR)
			// Add each verifier to our list of known verifiers :
			v.mu.Lock()
			for _, site_name := range site_names {
				if site_name != v.GetName() {
					v.otherVerifiers[site_name] = true
					v.nbOfVerifiers++
				}
			}
			v.mu.Unlock()

			v.mu.Lock()
			debug := "Nb of verifiers: " + strconv.Itoa(v.nbOfVerifiers)
			for key := range v.otherVerifiers {
				debug += " " + key
			}
			v.mu.Unlock()
			format.Display(format.Format_g(v.GetName(), "HandleMessage()", debug))
		}
	}

}

func (v *VerifierNode) removeAllExistenceOfReading(readingID string) {
	// Remove all occurrences of the readingID from the verifiedItemIDs map
	for senderID, itemIDs := range v.verifiedItemIDs {
		v.verifiedItemIDs[senderID] = utils.RemoveAllOccurrencesString(itemIDs, readingID)
	}
	// Remove the readingID from the pending items 
	for i, item := range v.pendingItems {
		if item.ReadingID == readingID {
			v.pendingItems = slices.Delete(v.pendingItems, i, i+1)
			break
		}
	}
	// Remove the readingID from the processing items 
	delete(v.processingItems, readingID)
}

// SendMessage sends a message to the control layer.
// Here is done the logic needed before and after sending the message
func (v *VerifierNode) SendMessage(msg string, toHandleMessageArgs...bool) {
	toHandleMessage := false
	if len(toHandleMessageArgs) > 0 {
		toHandleMessage = toHandleMessageArgs[0]
	}
	v.mu.Lock()
	v.vectorClock[v.nodeIndex]++
	serializedClock := utils.SerializeVectorClock(v.vectorClock)
	v_clk := v.clk
	v.mu.Unlock()

	if v.vectorClockReady {
		msg = format.Replaceval(msg, "vector_clock", serializedClock)
	}
	msg = format.Replaceval(msg, "clk", strconv.Itoa(v_clk))
	msg = format.Replaceval(msg, "id", v.GenerateUniqueMessageID())


	if toHandleMessage {
		v.ctrlLayer.HandleMessage(msg)
	} else {
		v.ctrlLayer.SendApplicationMsg(msg)
	}

	// Increment the number of messages sent
	// (used in ID generation, for next messages)
	v.mu.Lock()
	v.nbMsgSent++
	v.mu.Unlock()

}

// CheckUnverifiedItems tries to find and process unverified items
func (v *VerifierNode) CheckUnverifiedItems() {
	// Re-launch the search for unverified items
	v.findUnverifiedReadings()

	v.mu.Lock()
	l_pending := len(v.pendingItems)
	v.mu.Unlock()
	
	// No items to process
	if l_pending == 0 {
		return
	}
	
	// Get the next unverified item
	pendingItem := v.chooseReadingToVerify() // The item we want to process
	v.mu.Lock()
	itemID := pendingItem.ReadingID  // Its ID
	var is_processingItem bool = v.processingItems[itemID] // Are we already processing it?
	v.mu.Unlock()

	// Check if we're already processing this item
	if is_processingItem {
		return
	}
	
	// Mark that we're about to start processing this item
	v.mu.Lock()
	v.processingItems[itemID] = true
	v.mu.Unlock()
	
	// Try to acquire a distributed lock for this item
	go v.requestLock(pendingItem)
}

// chooseReadingToVerify selects a reading to verify from the pending items.
// No logic needed for now => just return the last one (as Users
// weight the most recent readings heavier).
func (v *VerifierNode) chooseReadingToVerify() models.Reading {
	v.mu.Lock()
	r := v.pendingItems[len(v.pendingItems)-1] // Get the last item
	v.mu.Unlock()
	return r
}

// findUnverifiedReadings checks all senders for readings that need verification
// and adds them to the pending items queue.
func (v *VerifierNode) findUnverifiedReadings() {
	v.mu.Lock()

	// First, delete all the pendingItems (because we will re-add them)
	v.pendingItems = make([]models.Reading, 0)

	// For each sender we're tracking, check its readings queue
	for senderID, readingsForThisSender := range v.recentReadings {
		// Get the verified eleemnts for this sender
		verifiedItemIDsForThisSender, exists := v.verifiedItemIDs[senderID]
		if !exists {
			// Never verified anything from this sender
			v.verifiedItemIDs[senderID] = make([]string, 0)
		} 
		
		// Check if there are new readings to verify
		// meaning they are in sender's queue but not in the verified indices 
		for _, reading := range readingsForThisSender {
			// Check if this reading is already verified 
			if !slices.Contains(verifiedItemIDsForThisSender, reading.ReadingID) {
				 _, isProcessing := v.processingItems[reading.ReadingID]// Check if we are already processing this item 
				 _, isLocked := v.lockedItems[reading.ReadingID] 	// Check if already locked 
				 _, hasGrantedLock := v.lockRequests[reading.ReadingID] // Check if already locked 
				if !isProcessing && !isLocked && !hasGrantedLock{
					v.pendingItems = append(v.pendingItems, reading) // Add to pending item 
				}
			}
		}

	}
	v.mu.Unlock()
}

// requestLock starts the distributed locking protocol for an item
func (v *VerifierNode) requestLock(reading models.Reading) {

	// If this is the only verifier, we can process the item directly
	if v.nbOfVerifiers == 0 { // No other verifiers
		go v.acquiredFullLockOnItem(reading.ReadingID)
		return
	}

	itemID := reading.ReadingID

	v.mu.Lock()
	// Initialize maps if needed
	// First, the lock requests = the requests we sent.
	if _, exists := v.lockRequests[itemID]; !exists {
		v.lockRequests[itemID] = make(map[string]int)
	}
	// v.lockRequests[itemID][v.GetName()] = v.vectorClock[v.nodeIndex] // Add our request to the map
	v.lockRequests[itemID][v.GetName()] = v.clk

	// Then, the lock replies = the replies we received
	// for this item. We reset replies for this item
	// as we are requesting a new lock.
	v.lockReplies[itemID] = make(map[string]bool)

	our_VC := utils.SerializeVectorClock(v.vectorClock)

	v.mu.Unlock()
	
	// Broadcast lock request to all other verifiers
	msg_id := v.GenerateUniqueMessageID()
	msg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", msg_id,
			"type", "lock_request",
			"sender_name", v.GetName(),
			"sender_name_source", v.GetName(),
			"sender_type", v.Type(),
			"destination", "verifiers",
			"item_id", itemID,
			"request_clk", strconv.Itoa(v.vectorClock[v.nodeIndex]),
			"clk", strconv.Itoa(v.clk),
			"vector_clock", our_VC,))
	v.SendMessage(msg)
}

// handleLockRequest processes a lock request from another verifier
func (v *VerifierNode) handleLockRequest(msg string) {
	// Extract information from the message
	senderID := format.Findval(msg, "sender_name_source")
	requestedSenderID := format.Findval(msg, "sender_id")
	indexStr := format.Findval(msg, "index")
	itemID := format.Findval(msg, "item_id")


	// msg_VC := format.RetrieveVectorClock(msg, len(v.vectorClock))
	msgVC_index_str := format.Findval(msg, "request_clk")
	msgVC_index, err := strconv.Atoi(msgVC_index_str)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "handleLockRequest()", "Error parsing vector clock: "+msgVC_index_str))
		return
	}
	msg_clk_str := format.Findval(msg, "clk")
	msg_clk, err := strconv.Atoi(msg_clk_str)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "handleLockRequest()", "Error parsing vector clock: "+msgVC_index_str))
		return
	}
	
	v.mu.Lock()
	
	// Initialize map if needed
	if _, exists := v.lockRequests[itemID]; !exists {
		v.lockRequests[itemID] = make(map[string]int)
	}
	
	// === Determine if we should grant the lock ===
	canGrant := true
	if v.processingItems[itemID] { // Check if we are currently processing this item
		canGrant = false
	} else if  _, exists := v.lockedItems[itemID]; exists {
		// Check if we already know that this item is locked in the lockedItems map
		canGrant = false
	} else if _, havePendingRequest := v.lockRequests[itemID][v.GetName()]; havePendingRequest {
		// Check if we have a pending request with higher priority
		ourClock := v.lockRequests[itemID][v.GetName()]
		if ourClock < msg_clk || (ourClock == msgVC_index && v.GetName() < senderID) {
			// In condition above, we are comparing two strings, GetName and senderID,
			// which are both the names of the verifiers. It is legal in Go.
			// Used to add arbitrary priority to the lock request.
			canGrant = false
		}
	}

	// Add the request to our lock requests if accepted 
	if canGrant {
		v.lockRequests[itemID][senderID] = msgVC_index // Add the request to our map
	}

	our_VC := utils.SerializeVectorClock(v.vectorClock)

	v.mu.Unlock()

	// Send reply
	replyMsg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", v.GenerateUniqueMessageID(),
			"type", "lock_reply",
			"sender_name", v.GetName(),
			"sender_name_source", v.GetName(),
			"sender_type", v.Type(),
			"destination", senderID,
			"vector_clock", our_VC,
			"clk","",
			"sender_id", requestedSenderID,
			"index", indexStr,
			"item_id", itemID,
			"granted", strconv.FormatBool(canGrant),
			))
	
	v.SendMessage(replyMsg)
}

// handleLockReply processes a lock reply from another verifier
func (v *VerifierNode) handleLockReply(msg string) {

	// Extract information from the message
	senderID := format.Findval(msg, "sender_name_source")
	itemID := format.Findval(msg, "item_id")
	grantedStr := format.Findval(msg, "granted")

	granted, err := strconv.ParseBool(grantedStr)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "handleLockReply()", "Error parsing granted value: "+grantedStr))
		return
	}
	
	v.mu.Lock()
	
	// Initialize map if needed
	if _, exists := v.lockReplies[itemID]; !exists {
		v.lockReplies[itemID] = make(map[string]bool)
	}
	
	// Record the reply
	v.lockReplies[itemID][senderID] = granted

	// Check if we have received replies from all verifiers:
	allReplied := true 
	canProcess := true
	for verifierID := range v.otherVerifiers {
		if _, replied := v.lockReplies[itemID][verifierID]; !replied {
			allReplied = false
			break
		} else {
			if !v.lockReplies[itemID][verifierID] {
				canProcess = false
				break
			}
		}
	}
	v.mu.Unlock()
	if allReplied && canProcess {
		go v.acquiredFullLockOnItem(itemID) // We can process the item

	} else if allReplied && !canProcess {
		go v.cancelLockRequest(itemID) // We can cancel the lock request 
	}
}


// cancelLockRequest cancels the lock request for an item
func (v *VerifierNode) cancelLockRequest(itemID string) {
	v.mu.Lock()
	// Remove the lock request from our map
	delete(v.lockRequests, itemID)
	
	// Remove the lock replies for this item
	delete(v.lockReplies, itemID)
	
	// Remove the processing status for this item
	delete(v.processingItems, itemID)
	
	// Remove the item from the pending items 
	for i, item := range v.pendingItems {
		if item.ReadingID == itemID {
			v.pendingItems = slices.Delete(v.pendingItems, i, i+1)
			break
		}
	}

	our_VC := utils.SerializeVectorClock(v.vectorClock)

	v.mu.Unlock()
	
	// Notify other verifiers that we are not processing this item anymore
	msg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", v.GenerateUniqueMessageID(),
			"type", "lock_request_cancelled",
			"sender_name", v.GetName(),
			"sender_name_source", v.GetName(),
			"sender_type", v.Type(),
			"destination", "verifiers",
			"vector_clock", our_VC,
			"clk","",
			"item_id", itemID))
	v.SendMessage(msg)
}
	


// acquiredFullLockOnItem is called when the current node has full lock on an item,
// meaning all other verifiers have granted the lock.
// It needs to 1) say to the other verifiers that we have the lock, while 2) process the item.
func (v *VerifierNode) acquiredFullLockOnItem(itemID string) {
	v.mu.Lock()
	our_VC := utils.SerializeVectorClock(v.vectorClock)
	v.mu.Unlock()

	// 1) Notify other verifiers that we have the lock 
	msg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", v.GenerateUniqueMessageID(),
			"type", "lock_acquired",
			"sender_name", v.GetName(),
			"sender_name_source", v.GetName(),
			"sender_type", v.Type(),
			"destination", "verifiers",
			"item_id", itemID,
			"clk","",
			"vector_clock", our_VC,
		),
	)
	v.SendMessage(msg)

	v.mu.Lock()

	// 2) Process the item
	v.verificationLocks[itemID] = true // Verifying this locks
	// No more lock requests for this item:
	delete(v.lockRequests, itemID)
	delete(v.lockReplies, itemID)
	v.lockedItems[itemID] = v.GetName() // Item locked by us

	v.mu.Unlock()
	
	go v.processItem(itemID) // Process the item
}

// handleLockAcquired processes a lock acquired message from another verifier
func (v *VerifierNode) handleLockAcquired(msg string) {
	// Extract information from the message
	itemID := format.Findval(msg, "item_id")
	senderID := format.Findval(msg, "sender_name_source")

	v.mu.Lock()

	v.lockedItems[itemID] = senderID // Mark that it has the lock 

	// Remove from pendingItems 
	for i, item := range v.pendingItems {
		if item.ReadingID == itemID {
			v.pendingItems = slices.Delete(v.pendingItems, i, i+1)
			break
		}
	}
	v.mu.Unlock()
}


// processItem verifies an item after acquiring the lock
func (v *VerifierNode) processItem(itemID string) {
	// Remove from pending:
	v.mu.Lock()
	for i, item := range v.pendingItems {
		var current_itemID string = item.ReadingID
		if current_itemID == itemID {
			v.pendingItems = slices.Delete(v.pendingItems, i, i+1)
			break
		}
	}
	v.mu.Unlock()
	
	// Process the item - perform the verification
	readingValue := v.getItemReadingValue(itemID)
	
	// Clamp to acceptable range
	clampedValue := v.clampToAcceptableRange(readingValue)
	
	// Update the item status in the database
	v.markItemAsVerified(itemID, clampedValue, "")

	// Release the lock
	v.releaseLock(itemID)
}

// getItemReadingValue retrieves the reading value for an item
func (v *VerifierNode) getItemReadingValue(itemID string) float32 {
	value, err := v.getValueFromReadingID(itemID)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "getItemReadingValue()", "Error getting value from item ID: "+itemID +".Err is:"+err.Error()))
		return 0.0
	}
	return value
}

// clampToAcceptableRange ensures the value is within the acceptable range
func (v *VerifierNode) clampToAcceptableRange(value float32) float32 {
	minValue := v.baseTemp[0]
	maxValue := v.baseTemp[1]

	// Wait (ie. processing time) before clamping if not 
	// in the range:
	if value < minValue - v.threshold || value > maxValue + v.threshold {
		time.Sleep(2 * time.Second)
	}
	
	// Take verifier's threshold into account:
	if value < minValue - v.threshold {
		return minValue - v.threshold
	}
	if value > maxValue + v.threshold {
		return maxValue + v.threshold 
	}

	return value
}

// markItemAsVerified updates the item status in the database
func (v *VerifierNode) markItemAsVerified(itemID string, value float32, verifier string) {
	// Remove from pending items if it was there
	v.mu.Lock()
	for i, item := range v.pendingItems {
		var current_itemID string = item.ReadingID
		if current_itemID == itemID {
			v.pendingItems = slices.Delete(v.pendingItems, i, i+1)
			break
		}
	}

	// Remove from processing items if it was there
	if _, exists := v.processingItems[itemID]; exists {
		delete(v.processingItems, itemID)
	}

	// By the time the verification is done, the item might have been gone (erased 
	// because new readings were received). If it is the case (ie. the itemID don't 
	// exists anymore), no need to update the verifiedItemIDs nor recentReadings:
	itemStillPresent, err:= v.isItemInReadings(itemID)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "markIAsVerified()", "Err in isItemInReadings(): "+ err.Error()))

	} else if itemStillPresent == false {
		v.mu.Unlock()
		return
	}

	senderID := strings.Split(itemID, "_")[0]
	// Update the verified item list for this sender
	if _, exists := v.verifiedItemIDs[senderID]; exists {
		v.verifiedItemIDs[senderID] = append(v.verifiedItemIDs[senderID], itemID)
	} else {
		v.verifiedItemIDs[senderID] = make([]string, 0)
		v.verifiedItemIDs[senderID] = append(v.verifiedItemIDs[senderID], itemID)
	}

	v.mu.Unlock()

	// Update the verified value in the `recentReadings` map:
	readingIndex, err := v.getReadingIndexFromSender_ID(senderID, itemID)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "markItemAsVerified()", "Error getting reading index: "+err.Error()))
		return
	}
	v.mu.Lock()
	v.recentReadings[senderID][readingIndex].IsVerified = true
	v.recentReadings[senderID][readingIndex].Temperature = value
	if verifier== "" {
		verifier = v.GetName()
	}
	v.recentReadings[senderID][readingIndex].VerifierID = verifier
	v.mu.Unlock()
	format.Display(format.Format_d(v.GetName(), "markItemAsVerified()", "Item "+itemID+" verified by "+verifier))
}

// releaseLock releases the lock on an item
func (v *VerifierNode) releaseLock(itemID string) {
	v.mu.Lock()
	// Remove our processing status:
	delete(v.processingItems, itemID)
	// No more need to keep this item in the verification locks:
	delete(v.verificationLocks, itemID)
	// Remove lock requests and associated replies:
	delete(v.lockRequests, itemID)
	delete(v.lockReplies, itemID)
	delete(v.lockedItems, itemID)
	v.mu.Unlock()
	
	// By the time the verification is done, the item might have been gone (erased 
	// because new readings were received). If it is the case (ie. the itemID don't 
	// exists anymore), no need to update the verifiedItemIDs nor recentReadings:
	itemStillPresent, err:= v.isItemInReadings(itemID)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "releaseLock()", "Err is isItemInReadings(): " + err.Error()))

	} else if itemStillPresent == false {
		return
	}

	// Notify other verifiers that we've released the lock.
	// Use this message to also send the new item value.
	itemValue, err := v.getValueFromReadingID(itemID)
	if err != nil {
		// If the item does not exist anymore, it means the queue erased it.
		// So other verifiers will do them same, so no need to release the lock. 
		// If they didn't received the new reading so didn't erase the item in question, 
		// keeping the lock here will reduce useless messaging.
		format.Display(format.Format_e(v.GetName(), "releaseLock()", "Error getting value from item ID: "+itemID +".Err is:"+err.Error()))
		return
	}


	v.mu.Lock()
	our_VC := utils.SerializeVectorClock(v.vectorClock)
	releaseMsgID := v.GenerateUniqueMessageID()
	releaseMsg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", releaseMsgID,
			"type", "lock_release_and_verified_value",
			"sender_name", v.GetName(),
			"sender_name_source", v.GetName(),
			"sender_type", v.Type(),
			"destination", "verifiers",
			"item_id", itemID,
			"content_type","verified_value",
			"content_value", strconv.FormatFloat(float64(itemValue), 'f', 2, 32),
			"clk","",
			"vector_clock", our_VC,
		))

	v.mu.Unlock()

	v.SendMessage(releaseMsg)
}


// handleLockRelease processes a lock release message from another verifier
func (v *VerifierNode) handleLockRelease(msg string) {
	// Extract information from the message
	itemID := format.Findval(msg, "item_id")
	verifiedValueStr := format.Findval(msg, "content_value")
	verifiedValue, err := strconv.ParseFloat(verifiedValueStr, 32)
	if err != nil {
		format.Display(format.Format_e(v.GetName(), "handleLockRelease()", "Error parsing verified value: "+verifiedValueStr))
		return
	}

	
	v.mu.Lock()
	// Clean up any requests or replies for this item:
	delete(v.lockRequests, itemID)
	delete(v.lockReplies, itemID)
	delete(v.lockedItems, itemID)
	v.mu.Unlock()

	// Update verified value
	var verifier string = format.Findval(msg, "sender_name_source")
	v.markItemAsVerified(itemID, float32(verifiedValue), verifier)
}

// ====================== MODEL `READING` LOGIC ======================
// Helper function for using the `Reading` struct

// getValueFromReadingID retrieves the temperature value from a reading ID
// Assumption: itemID are in the format "senderID_index" 
func (v *VerifierNode) getValueFromReadingID(itemID string) (float32, error) {
	// Split the item ID into sender ID and index
	parts := strings.Split(itemID, "_")
	if len(parts) != 2 {
		return 0.0, fmt.Errorf("invalid item ID format")
	}
	senderID := parts[0]
	
	// Check if the sender ID exists in the recent readings 
	// (function might be called with wrong sender ID). And then 
	// find the reading for this sender.
	v.mu.Lock()
	queue, exists := v.recentReadings[senderID]
	v.mu.Unlock()
	if !exists {
		return 0.0, fmt.Errorf("No sender <" + senderID  + "> in v.recentReadings.")
	} else {
		// Find the reading for this sender
		for _, rItem := range queue {
			if rItem.ReadingID == itemID {
				// Return the temperature value of the item
				return rItem.Temperature, nil
			}
		}
	}
	
	return 0.0, fmt.Errorf("item " + itemID + " from sender " + senderID + " not found in v.recentReadings")
}
func (v *VerifierNode) getReadingIndexFromSender_ID(senderID string, itemID string) (int, error) {
	// Check if the sender ID exists in the recent readings 
	v.mu.Lock()
	queue, exists := v.recentReadings[senderID]
	v.mu.Unlock()
	
	if !exists {
		return -1, fmt.Errorf("item not found")
	}
	
	// Find the index of the item ID in the queue
	for i, reading := range queue {
		if reading.ReadingID == itemID {
			return i, nil
		}
	}
	
	return -1, fmt.Errorf("item not found")
}

// isItemInReadings returns true if the given ItemID exists 
// for its associated sender (fetched within the itemID).
// It is used when operation on a models.Reading, as it might
// have been erased if FIFO was full.
func (v *VerifierNode) isItemInReadings(itemID string) (bool, error) {
	parts := strings.Split(itemID, "_")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid item ID format")
	}
	senderID := parts[0]
	for _, reading := range v.recentReadings[senderID] {
		if reading.ReadingID == itemID {
			return true, nil
		}
	}
	return false, nil
}
// ====================== END OF MODEL `READING` LOGIC ======================

func (v *VerifierNode) logReceivedReading(sender string, temperature float64, stateCode int) {
	filename := "node_" + v.ID() + ".log"

	line := "/" + "timestamp=" + time.Now().Format(time.RFC3339) +
		"/from=" + sender +
		"/temp=" + strconv.FormatFloat(temperature, 'f', 2, 64) +
		"/state=" + strconv.Itoa(stateCode) + "\n"

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(line)
	}
}
func (n* VerifierNode) GetLocalState() string {
	n.mu.Lock()
	snap_content := ""
	for senderID, readings := range n.recentReadings {
		snap_content += senderID + ": ["
		for _, reading := range readings {
			snap_content += reading.ReadingID + ","+strconv.FormatFloat(float64(reading.Temperature), 'f', 2, 32) + "," + strconv.Itoa(reading.Clock) + "," + strconv.FormatBool(reading.IsVerified) + "," + reading.VerifierID + ";"
		}
		snap_content += "]"
		snap_content += utils.PearD_SITE_SEPARATOR
	}
	n.mu.Unlock()
	return snap_content
}
