package node

import (
	"fmt"
	"time"

	// "bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	// "os"    // Use for the bufio reader: reads from os stdin
	//  "strings"
	"distributed_system/format"
	"distributed_system/utils"
	"log" // Added for logging errors
	"strconv"
)

// UserNode represents a user in the system
type UserNode struct {
	BaseNode
	// store           *storage.DataStore
	predictionModel    string
	predictionWindow   time.Duration
	predictionInterval time.Duration
	lastPrediction     *float32             // pointer to allow nil value at start
	recentReadings     map[string][]float32 // FIFO queue per sender
}

// NewUserNode creates a new user node
func NewUserNode(id string, model string, window time.Duration) *UserNode {
	return &UserNode{
		BaseNode:           NewBaseNode(id, "user"),
		predictionModel:    model,
		predictionWindow:   window,
		predictionInterval: 3 * time.Second, // Make new predictions hourly
		lastPrediction:     nil,
		recentReadings:     make(map[string][]float32),
	}
}

// Start begins the verifier's operation
func (u *UserNode) Start() error {
	format.Display(format.Format_d("node_user.go", "Start()", "Starting user node "+u.GetName()))

	u.isRunning = true

	// ⚡ Lancer une requête snapshot après un petit délai
	go func() {
		time.Sleep(5 * time.Second) // attendre que tout soit initialisé
		format.Display(format.Format_d("Start()", u.GetName(), "⏳ 5s écoulées, envoi de la requête snapshot..."))
		u.RequestSnapshot()
		format.Display(format.Format_d("Start()", u.GetName(), "📤 snapshot_request envoyé depuis "+u.GetName()))
	}()

	return nil
}

func (u *UserNode) InitVectorClockWithSites(sites []string) {
	u.vectorClock = make([]int, len(sites))
	u.nodeIndex = utils.FindIndex(u.GetControlName(), sites)
}

// HandleMessage processes incoming messages from control layer
func (u *UserNode) HandleMessage(channel chan string) {

	for msg := range channel {
		var msg_clock string = format.Findval(msg, "clk", u.GetName())

		// Update Lamport clock based on received message
		msg_clock_int, _ := strconv.Atoi(msg_clock)
		u.clock = utils.Synchronise(u.clock, msg_clock_int)

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
			queue, exists := u.recentReadings[msg_sender]
			if !exists {
				queue = make([]float32, 0, utils.VALUES_TO_STORE)
			}

			// Add to FIFO queue for this sender
			if len(queue) >= utils.VALUES_TO_STORE {
				// Remove the oldest element (slice trick)
				queue = queue[1:]
			}
			queue = append(queue, float32(readingVal))
			u.recentReadings[msg_sender] = queue // Update the map

			// Optional: Log the current queue state for debugging
			log.Printf("%s: Current readings queue for %s: %v", u.GetName(), msg_sender, queue)

		case "snapshot_verified":
			sensor := format.Findval(msg, "sender_name_source", u.GetName())
			snapshot := format.Findval(msg, "content_value", u.GetName())
			vc := format.Findval(msg, "vector_clock", u.GetName())

			// Couleurs ANSI
			green := "\033[32m"
			blue := "\033[34m"
			yellow := "\033[33m"
			reset := "\033[0m"
			bold := "\033[1m"

			fmt.Println()
			fmt.Println(bold + green + "📦  Snapshot Vérifié Reçu" + reset)
			fmt.Println(bold + "──────────────────────────────────────────────" + reset)
			fmt.Printf("📍 %sCapteur%s        : %s%s%s\n", blue, reset, yellow, sensor, reset)
			fmt.Printf("🧪 %sValeurs%s        : %s%s%s\n", blue, reset, yellow, snapshot, reset)
			fmt.Printf("🕒 %sHorloge vectorielle%s : %s%s%s\n", blue, reset, yellow, vc, reset)
			fmt.Println(bold + "──────────────────────────────────────────────" + reset)
			fmt.Println()
		}

	}
}

func (u *UserNode) RequestSnapshot() {
	u.clock += 1                    // Incrément de l'horloge logique
	u.vectorClock[u.nodeIndex] += 1 // Incrément de l'horloge vectorielle locale

	msgID := u.GenerateUniqueMessageID()
	msg := format.Msg_format_multi(format.Build_msg_args(
		"id", msgID,
		"type", "snapshot_request",
		"sender_name", u.GetName(),
		"sender_name_source", u.GetName(),
		"sender_type", u.Type(),
		"destination", "applications", // pour que le msg soit traité par tous les capteurs via control layers
		"clk", strconv.Itoa(u.clock),
		"vector_clock", utils.SerializeVectorClock(u.vectorClock),
		"content_type", "request_type",
		"content_value", "snapshot_start",
	))

	err := u.ctrlLayer.SendApplicationMsg(msg)
	if err != nil {
		log.Printf("Erreur lors de l'envoi du snapshot request : %v", err)
	} else {
		format.Display(format.Format_d(
			u.GetName(), "RequestSnapshot()",
			"Sent snapshot request to all sensors",
		))
	}
}
