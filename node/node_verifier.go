package node

import (
	"fmt"
	"strconv"
	"time"

	// "bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"os" // Use for the bufio reader: reads from os stdin
	// "strings"
	"distributed_system/format"
	"distributed_system/utils"
	"log" // Added for logging errors
)

// VerifierNode represents a verifier in the system
type VerifierNode struct {
	BaseNode
	// store              *storage.DataStore
	processingCapacity int
	threshold          float64
	verificationLocks  map[string]bool            // Maps day IDs to lock status
	lockRequests       map[string]map[string]int  // Maps day IDs to node IDs and timestamps
	lockReplies        map[string]map[string]bool // Maps day IDs to node IDs and reply status
	otherVerifiers     map[string]bool            // Set of other verifier node IDs
	recentReadings     map[string][]float32       // FIFO queue per sender
}

// NewVerifierNode creates a new verifier node
func NewVerifierNode(id string, capacity int, threshold float64) *VerifierNode {
	return &VerifierNode{
		BaseNode: NewBaseNode(id, "verifier"),
		// clock:              lamport.NewClock(),
		processingCapacity: capacity,
		threshold:          threshold,
		verificationLocks:  make(map[string]bool),
		lockRequests:       make(map[string]map[string]int),
		lockReplies:        make(map[string]map[string]bool),
		otherVerifiers:     make(map[string]bool),
		recentReadings:     make(map[string][]float32),
	}
}

// Start begins the verifier's operation
func (v *VerifierNode) Start() error {
	format.Display(format.Format_d("Start()", "node_verifier.go", "Starting verifier node "+v.GetName()))

	v.isRunning = true

	return nil
}

func (v *VerifierNode) InitVectorClockWithSites(sites []string) {
	v.vectorClock = make([]int, len(sites))
	v.nodeIndex = utils.FindIndex(v.GetControlName(), sites)
}

// HandleMessage processes incoming messages from control app
func (v *VerifierNode) HandleMessage(channel chan string) {

	for msg := range channel {
		var msg_clock string = format.Findval(msg, "clk", v.GetName())

		// Update Lamport clock based on received message
		msg_clock_int, _ := strconv.Atoi(msg_clock)
		v.clock = utils.Synchronise(v.clock, msg_clock_int)

		var msg_type string = format.Findval(msg, "type", v.GetName())
		var msg_content_value string = format.Findval(msg, "content_value", v.GetName())
		var msg_sender string = format.Findval(msg, "sender", v.GetName()) // Get sender ID

		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store for the specific sender
			format.Display(format.Format_d(
				v.GetName(), "HandleMessage",
				v.GetName()+" received the new reading <"+msg_content_value+"> from "+msg_sender))

			// Parse the reading value
			readingVal, err := strconv.ParseFloat(msg_content_value, 32)
			if err != nil {
				log.Printf("%s: Error parsing reading value '%s': %v", v.GetName(), msg_content_value, err)
				continue // Skip this message if parsing fails
			}

			// Get or create the queue for the sender
			queue, exists := v.recentReadings[msg_sender]
			if !exists {
				queue = make([]float32, 0, utils.VALUES_TO_STORE)
			}

			// Add to FIFO queue for this sender
			if len(queue) >= utils.VALUES_TO_STORE {
				// Remove the oldest element (slice trick)
				queue = queue[1:]
			}
			queue = append(queue, float32(readingVal))
			v.recentReadings[msg_sender] = queue // Update the map

			// Optional: Log the current queue state for debugging
			log.Printf("%s: Current readings queue for %s: %v", v.GetName(), msg_sender, queue)

			// Définir le code d'état
			var stateCode int
			if readingVal > v.threshold || readingVal < -v.threshold {
				stateCode = 2 // invalid
			} else {
				stateCode = 1 // valid
			}

			// Sauvegarde dans le fichier log
			v.logReceivedReading(msg_sender, readingVal, stateCode)

		case "snapshot_response":
			fmt.Println("[VerifierNode] ✅ snapshot_response reçu")
			// 🔁 Mise à jour du vector clock
			vcStr := format.Findval(msg, "vector_clock", v.GetName())
			recvVC, err := utils.DeserializeVectorClock(vcStr)
			if err == nil {
				for i := 0; i < len(v.vectorClock); i++ {
					v.vectorClock[i] = utils.Max(v.vectorClock[i], recvVC[i])
				}
				v.vectorClock[v.nodeIndex] += 1
			}

			// Extraire infos
			snapshotData := format.Findval(msg, "content_value", v.GetName())
			sensorID := format.Findval(msg, "sender_name", v.GetName())
			originalRequester := format.Findval(msg, "sender_name_source", v.GetName()) // le UserNode !

			//(plus tard : vérifier cohérence)
			format.Display(format.Format_d(v.GetName(), "HandleMessage()", "Received snapshot from "+sensorID+" → "+snapshotData))

			// Construire message de réponse
			msgID := v.GenerateUniqueMessageID()
			response := format.Msg_format_multi(format.Build_msg_args(
				"id", msgID,
				"type", "snapshot_verified",
				"sender_name", v.GetName(),
				"sender_name_source", v.GetName(),
				"sender_type", v.Type(),
				"destination", originalRequester, // vers UserNode qui a demandé
				"clk", strconv.Itoa(v.clock),
				"vector_clock", utils.SerializeVectorClock(v.vectorClock),
				"content_type", "status",
				"content_value", "valid snapshot from "+sensorID,
			))

			// Envoi vers UserNode
			v.ctrlLayer.SendApplicationMsg(response)

		}
	}

}

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
