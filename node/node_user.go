package node

import (
	"distributed_system/format"
	"distributed_system/models"
	"distributed_system/utils"
	"fmt"
	"log" // Added for logging errors
	"sort"
	"strconv"
	"strings"

	// Server and JSON libraries:
	"encoding/json"
	"net/http"
	"time"
)

// UserNode represents a user in the system
type UserNode struct {
	BaseNode
	predictionFunc     func(values []float32, decay float32) float32
	model              string                      // Model type (linear or exponential)
	decayFactor        float32                     // Decay factor in some prediction functions
	lastPrediction     *float32                    // pointer to allow nil value at start
	recentReadings     map[string][]models.Reading // FIFO queue per sender
	recentPredictions  map[string][]float32        // FIFO queue per sender of made predictions
	verifiedItemIDs    map[string][]string         // Tracks the verified item for each sender by their ID
	httpServer         *http.Server                // HTTP server for web UI
	port               int                         // HTTP server port
	snapshotInProgress bool
	down               bool // True when disconnected from the network
}

// NewUserNode creates a new user node
func NewUserNode(id string, model string, port int) *UserNode {

	// Set the prediction function based on the model type
	var predFunction func(values []float32, decay float32) float32
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
		model:              model,
		decayFactor:        decayFactor,
		lastPrediction:     nil,
		recentReadings:     make(map[string][]models.Reading),
		recentPredictions:  make(map[string][]float32),
		verifiedItemIDs:    make(map[string][]string),
		port:               port,
		snapshotInProgress: false,
		down:               false,
	}
}

// Start begins the verifier's operation
func (u *UserNode) Start() error {
	format.Display(format.Format_d("node_user.go", "Start()", "Starting user node "+u.GetName()+" on port "+strconv.Itoa(u.port)))

	// Start the HTTP server for web UI
	go u.startWebServer()

	go func() {
		u.mu.Lock()
		vcReady := u.vectorClockReady
		u.mu.Unlock()
		for !vcReady {
			time.Sleep(500 * time.Millisecond) // Wait until vector clock is ready
			u.mu.Lock()
			vcReady = u.vectorClockReady
			u.mu.Unlock()
		}
		u.isRunning = true
	}()
	return nil
}

func (u *UserNode) InitVectorClockWithSites(sites []string) {
	u.mu.Lock()
	u.vectorClock = make([]int, len(sites))
	u.nodeIndex = utils.FindIndex(u.ctrlLayer.GetName(), sites)
	u.vectorClockReady = true
	u.mu.Unlock()
}

// HandleMessage processes incoming messages from control layer
func (u *UserNode) HandleMessage(channel chan string) {

	for msg := range channel {
		u.mu.Lock()

		if !u.isRunning {
			u.mu.Unlock()
			continue
		}
		if u.down {
			u.mu.Unlock()
			return
		}

		rec_clk_str := format.Findval(msg, "clk")
		rec_clk, _ := strconv.Atoi(rec_clk_str)
		u.clk = utils.Synchronise(u.clk, rec_clk)
		clk_int := u.clk
		if u.vectorClockReady {
			recVC := format.RetrieveVectorClock(msg, len(u.vectorClock))
			vectorClock, err := utils.SynchroniseVectorClock(u.vectorClock, recVC, u.nodeIndex)
			u.vectorClock = vectorClock
			if err != nil {
				if len(u.vectorClock) == 0 || len(u.vectorClock) > len(recVC) {
					u.vectorClock = recVC
				}
			}
		}
		u.mu.Unlock()

		var msg_type string = format.Findval(msg, "type")
		var msg_content_value string = format.Findval(msg, "content_value")
		// var msg_sender string = format.Findval(msg, "sender_name")

		itemID := format.Findval(msg, "item_id")
		sensor_name := strings.Split(itemID, "_")[0]

		switch msg_type {
		case "new_reading":
			// Add the new reading to our local store for the specific sender
			format.Display(format.Format_d(
				"node_user.go", "HandleMessage()",
				u.GetName()+" received the new reading <"+msg_content_value+"> from "+sensor_name))

			// Parse the reading value
			readingVal, err := strconv.ParseFloat(msg_content_value, 32)

			if err != nil {
				log.Printf("%s: Error parsing reading value '%s': %v", u.GetName(), msg_content_value, err)
				continue // Skip this message if parsing fails
			}

			// Get or create the queue for the sender
			u.mu.Lock()
			queue, exists := u.recentReadings[sensor_name]
			if !exists {
				queue = make([]models.Reading, 0, utils.VALUES_TO_STORE)
			} else {
				// If the queue exist, maybe we already have this reading (verifier answer received before this new_reading)
				// So make sure we don't add it again:
				// Check if the reading already exists in the queue
				for _, reading := range queue {
					if reading.ReadingID == format.Findval(msg, "item_id") {
						u.mu.Unlock()
						continue
					}
				}
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
				ReadingID:   format.Findval(msg, "item_id"),
				Temperature: float32(readingVal),
				Clock:       clk_int,
				SensorID:    sensor_name,
				IsVerified:  false,
			})
			u.recentReadings[sensor_name] = queue // Update the map
			u.mu.Unlock()

			u.processDatabse()
		case "lock_release_and_verified_value":
			go u.handleLockRelease(msg)

		case "snapshot_request":
			format.Display(format.Format_d(
				u.GetName(), "HandleMessage()",
				"📦 snapshot_request reçu"))

			// Create answer
			msgID := u.GenerateUniqueMessageID()
			u.mu.Lock()
			response := format.Msg_format_multi(format.Build_msg_args(
				"id", msgID,
				"type", "snapshot_response",
				"sender_name", u.GetName(),
				"sender_name_source", u.GetName(),
				"sender_type", u.Type(),
				"destination", format.Findval(msg, "sender_name_source"),
				"clk", strconv.Itoa(u.clk),
				"vector_clock", utils.SerializeVectorClock(u.vectorClock),
				"content_type", "snapshot_data",
				"content_value", "[]", // Empty for now
			))
			u.mu.Unlock()

			format.Display(format.Format_d(u.GetName(), "HandleMessage()", "Sending snapshot_response"))
			if u.ctrlLayer.id != "0_control" {
				// v.ctrlLayer.SendApplicationMsg(response)
				u.SendMessage(response)
			} else {
				// v.ctrlLayer.HandleMessage(response)
				u.SendMessage(response, true)
			}

			u.mu.Lock()
			u.nbMsgSent++
			u.mu.Unlock()

		}

	}
}

func (v *UserNode) SendMessage(msg string, toHandleMessageArgs ...bool) {
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
		// v.ctrlLayer.HandleMessage(msg)
		v.channel_to_ctrl <- msg
	} else {
		v.ctrlLayer.SendApplicationMsg(msg)
	}

	// Increment the number of messages sent
	// (used in ID generation, for next messages)
	v.mu.Lock()
	v.nbMsgSent++
	v.mu.Unlock()

}

// handleLockRelease processes a lock release message from a verifier
// which also contains the verified value.
func (n *UserNode) handleLockRelease(msg string) {
	// Extract information from the message
	itemID := format.Findval(msg, "item_id")
	sensorID := strings.Split(itemID, "_")[0]
	verifier := format.Findval(msg, "verified_by")
	verifiedValueStr := format.Findval(msg, "content_value")
	verifiedValue, err := strconv.ParseFloat(verifiedValueStr, 32)
	if err != nil {
		format.Display(format.Format_e(n.GetName(), "handleLockRelease()", "Error parsing verified value: "+verifiedValueStr))
		return
	}

	n.mu.Lock()

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
	if !isItemInReadings {
		// Maybe the item is most recent than the latest we have => verifier sent the msg before we receive the reading from the sensor.
		// We this add this item to the recentReadings:

		need_to_add := false

		// 1: find the latest reading for this sensor
		_, exists := n.recentReadings[sensorID]
		if !exists {
			need_to_add = true
		} else {
			latestReading := n.recentReadings[sensorID][len(n.recentReadings[sensorID])-1]
			latestReadig_clk, _ := strconv.Atoi(strings.Split(latestReading.ReadingID, "_")[1])

			// 2: Check if the reading is more recent than the latest we have
			item_clk := strings.Split(itemID, "_")[1]
			item_clk_int, err := strconv.Atoi(item_clk)
			if err != nil {
				log.Printf("%s: Error parsing clock from itemID '%s': %v", n.GetName(), itemID, err)
				n.mu.Unlock()
				return
			}

			if item_clk_int > latestReadig_clk {
				need_to_add = true
			} else {
				need_to_add = false
			}
		}

		if need_to_add {
			// Get or create the queue for the sender
			n.mu.Lock()
			queue, exists := n.recentReadings[sensorID]
			if !exists {
				queue = make([]models.Reading, 0, utils.VALUES_TO_STORE)
			} else {
				// If the queue exist, maybe we already have this reading (verifier answer received before this new_reading)
				// So make sure we don't add it again:
				// Check if the reading already exists in the queue
				for _, reading := range queue {
					if reading.ReadingID == format.Findval(msg, "item_id") {
						n.mu.Unlock()
						continue
					}
				}
			}

			// Add to FIFO queue for this sender
			if len(queue) >= utils.VALUES_TO_STORE {

				// Remove all occurrences of the readingID from the verifiedItemIDs map
				for senderID, itemIDs := range n.verifiedItemIDs {
					n.verifiedItemIDs[senderID] = utils.RemoveAllOccurrencesString(itemIDs, queue[0].ReadingID)
				}

				// Remove the oldest element (slice trick)
				queue = queue[1:]
			}
			queue = append(queue, models.Reading{
				ReadingID:   format.Findval(msg, "item_id"),
				Temperature: float32(verifiedValue),
				Clock:       n.clk,
				SensorID:    sensorID,
				IsVerified:  false,
			})
			n.recentReadings[sensorID] = queue // Update the map
			n.mu.Unlock()

		} else {

			format.Display_w(n.GetName(), "handleLockRelease()", "Item "+itemID+" not found in recent readings for sensor "+sensorID+". Skipping verification update.")
			n.mu.Unlock()
			return
		}
	}

	// Update the verified item list for this sender
	if _, exists := n.verifiedItemIDs[sensorID]; exists {
		n.verifiedItemIDs[sensorID] = append(n.verifiedItemIDs[sensorID], itemID)
	} else {
		n.verifiedItemIDs[sensorID] = make([]string, 0)
		n.verifiedItemIDs[sensorID] = append(n.verifiedItemIDs[sensorID], itemID)
	}
	n.mu.Unlock()

	// Update verified value:
	n.mu.Lock()
	n.recentReadings[sensorID][readingIndex].IsVerified = true
	n.recentReadings[sensorID][readingIndex].Temperature = float32(verifiedValue)
	n.recentReadings[sensorID][readingIndex].VerifierID = verifier
	n.mu.Unlock()
	n.processDatabse()
}

func (n *UserNode) processDatabse() {

	// One prediction per sensor:
	n.mu.Lock()
	recentReadings := n.recentReadings
	n.mu.Unlock()
	for sensor, readings := range recentReadings {
		var readingValues []float32 = make([]float32, len(readings))
		for _, r := range readings {
			readingValues = append(readingValues, r.Temperature)
		}
		prediction := n.predictionFunc(readingValues, n.decayFactor)
		// Get or create the queue for the sender

		n.mu.Lock()
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
		n.mu.Unlock()
	}
	// n.printDatabase()
}

func (n *UserNode) printDatabase() {
	var debug string = n.GetName() + " database:\n"

	n.mu.Lock()

	// 1. Sort and print readings
	sensorNames := make([]string, 0, len(n.recentReadings))
	for sensor := range n.recentReadings {
		sensorNames = append(sensorNames, sensor)
	}
	sort.Strings(sensorNames) // Alphabetical sort

	for _, sensor := range sensorNames {
		debug += sensor + "\n"
		for _, r := range n.recentReadings[sensor] {
			debug += "	" + r.ReadingID + "  " + fmt.Sprintf("%.2f", r.Temperature) + " verifier:" + r.VerifierID + "\n"
		}
	}

	// 2. Sort and print predictions
	predictionSensorNames := make([]string, 0, len(n.recentPredictions))
	for sensor := range n.recentPredictions {
		predictionSensorNames = append(predictionSensorNames, sensor)
	}
	sort.Strings(predictionSensorNames)

	for _, sensor := range predictionSensorNames {
		prediction := n.recentPredictions[sensor][len(n.recentPredictions[sensor])-1] // latest prediction
		debug += "Latest prediction for " + sensor + ": " + strconv.FormatFloat(float64(prediction), 'f', -1, 32) + "\n"
	}

	n.mu.Unlock()
	format.Display(format.Format_e(n.GetName(), "handleLockRelease()", debug))

}

// startWebServer initializes and starts the HTTP server for the web UI
func (u *UserNode) startWebServer() {
	mux := http.NewServeMux()

	// Serve the dashboard HTML page
	mux.HandleFunc("/", u.handleDashboard)

	// API endpoint to get data in JSON format
	mux.HandleFunc("/api/data", u.handleAPIData)

	// API endpoint to take a snapshot
	mux.HandleFunc("/api/snapshot", u.handleSnapshot)

	// API endpoint to log out
	mux.HandleFunc("/api/logout", u.handleLogout)

	// Serve static files if needed
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	u.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", u.port),
		Handler: mux,
	}

	format.Display(format.Format_d("node_user.go", "startWebServer()",
		fmt.Sprintf("%s starting web server on port %d", u.GetName(), u.port)))

	if err := u.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("%s: Error starting web server: %v", u.GetName(), err)
	}
}

func (u *UserNode) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !u.isRunning {
		http.Error(w, "User node is not running", http.StatusInternalServerError)
		return
	}

	// format.Display_w(u.GetName(), "handleSnapshot()", "Taking snapshot...")
	u.mu.Lock()
	u.snapshotInProgress = true
	snapshotInProgress := true
	u.mu.Unlock()
	u.ctrlLayer.RequestSnapshot()

	timeWaited := 0
	maxWait := 3 * time.Second // Maximum wait time for snapshot to complete
	for snapshotInProgress {
		time.Sleep(100 * time.Millisecond) // Wait for snapshot to complete
		u.mu.Lock()
		snapshotInProgress = u.snapshotInProgress
		u.mu.Unlock()
		timeWaited += 100
		if timeWaited >= int(maxWait.Milliseconds()) {
			break
		}
	}

	if !snapshotInProgress {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Snapshot taken successfully"))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Snapshot could not be taken in time"))
	}
}

func (n *UserNode) SetSnapshotInProgress(inProgress bool) {
	n.mu.Lock()
	n.snapshotInProgress = inProgress
	n.mu.Unlock()
}

func (u *UserNode) handleLogout(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.isRunning = false // stop user processing

	format.Display_e(u.GetName(), "handleLogout()", "User node "+u.GetName()+" is logging out...")

	u.ctrlLayer.SendLogoutAnnouncement()
	u.ctrlLayer.SendConnectNeighbors()

	u.down = true // Set the node as down

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Logout called"))
}

// handleDashboard serves the HTML dashboard
func (u *UserNode) handleDashboard(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + u.GetName() + ` (` + u.model + `) Dashboard</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f5;
        }
        h1, h2 {
            color: #333;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background-color: white;
            padding: 20px;
            border-radius: 5px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
        }
        .sensor-container {
            margin-bottom: 20px;
            padding: 15px;
            border: 1px solid #ddd;
            border-radius: 4px;
            background-color: #f9f9f9;
        }
        .reading {
            padding: 8px;
            margin: 5px 0;
            background-color: white;
            border: 1px solid #eee;
            border-radius: 3px;
        }
        .reading.verified {
            border-left: 4px solid #4CAF50;
        }
        .prediction {
            margin-top: 10px;
            padding: 8px;
            background-color: #e1f5fe;
            border-radius: 3px;
            font-weight: bold;
        }
        .timestamp {
            color: #666;
            font-size: 0.8em;
            text-align: right;
        }
        .refresh-button {
            padding: 10px 15px;
            background-color: #4CAF50;
            color: white;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 1em;
            margin-bottom: 20px;
        }
        .refresh-button:hover {
            background-color: #45a049;
        }

    .chart-container {
	height: 550px;
	margin-bottom: 30px;
	padding: 15px;
	background-color: white;
	border: 1px solid #ddd;
	border-radius: 4px;
    }

    .verification-stats {
        margin-bottom: 30px;
        padding: 15px;
        background-color: white;
        border: 1px solid #ddd;
        border-radius: 4px;
    }
    
    .stats-container {
        display: flex;
        flex-wrap: wrap;
        gap: 15px;
        margin-top: 10px;
    }
    
    .stat-card {
        flex: 1;
        min-width: 200px;
        padding: 15px;
        border-radius: 4px;
        box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        text-align: center;
    }
    
    .stat-value {
        font-size: 2em;
        font-weight: bold;
        margin: 10px 0;
    }
    
    .stat-label {
        font-size: 1.2em;
        color: #555;
    }
    
    .stat-percentage {
        font-size: 1em;
        color: #666;
    }
    </style>
</head>
<body>
    <div class="container">
        <h1>` + u.GetName() + ` (` + u.model + `) Dashboard</h1>
        <button class="refresh-button" onclick="fetchData()">Refresh Data</button>
        <button class="refresh-button" onclick="takeSnapshot()">📷 Take snapshot</button>
		<button class="refresh-button" id="logout-button" onclick="logout()">🚪 Déconnexion</button>
        <div id="last-updated" class="timestamp"></div>
	<div class="chart-container">
	    <h2>Recent Predictions</h2>
	    <canvas id="predictionsChart"></canvas>
	</div>
	<div class="verification-stats">
	    <h2>Verification Statistics</h2>
	    <div class="stats-container" id="verification-stats-container">
		<!-- Stats will be populated here -->
	    </div>
	</div>
        <div id="sensors-data"></div>
    </div>

	<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.9.1/chart.min.js"></script>
    <script>

	 // Global chart variable so we can update it
    let predictionsChart = null;

    function updateChart(data) {
        const ctx = document.getElementById('predictionsChart').getContext('2d');
        
        // Generate colors for each sensor
        const colorPalette = [
            'rgba(255, 99, 132, 1)',   // red
            'rgba(54, 162, 235, 1)',   // blue
            'rgba(255, 206, 86, 1)',   // yellow
            'rgba(75, 192, 192, 1)',   // teal
            'rgba(153, 102, 255, 1)',  // purple
            'rgba(255, 159, 64, 1)',   // orange
            'rgba(199, 199, 199, 1)',  // gray
            'rgba(83, 102, 255, 1)',   // indigo
            'rgba(255, 99, 255, 1)',   // pink
            'rgba(99, 255, 132, 1)'    // light green
        ];
        
        // Prepare datasets from predictions
        const datasets = [];
        let colorIndex = 0;
        
        for (const [sensorId, predictions] of Object.entries(data.predictions)) {
            if (predictions && predictions.length > 0) {
                // Generate indices for X-axis (0, 1, 2, ...)
                const indices = Array.from({ length: predictions.length }, (_, i) => i);
                
                datasets.push({
                    label: sensorId,
                    data: predictions,
                    borderColor: colorPalette[colorIndex % colorPalette.length],
                    backgroundColor: colorPalette[colorIndex % colorPalette.length].replace('1)', '0.2)'),
                    tension: 0.1,
                    pointRadius: 3
                });
                
                colorIndex++;
            }
        }
        
        // If we already have a chart, destroy it before creating a new one
        if (predictionsChart) {
            predictionsChart.destroy();
        }
        
        // Create new chart
        predictionsChart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: datasets.length > 0 ? datasets[0].data.map((_, i) => i) : [],
                datasets: datasets
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    y: {
                        title: {
                            display: true,
                            text: 'Prediction Value'
                        }
                    },
                    x: {
                        title: {
                            display: true,
                            text: 'Prediction Index'
                        }
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: 'Sensor Predictions Over Time'
                    },
                    legend: {
                        position: 'top',
                    }
                }
            }
        });
    }

    // function to update verification stats
    function updateVerificationStats(data) {
        const statsContainer = document.getElementById('verification-stats-container');
        statsContainer.innerHTML = '';
        
        // Calculate verification stats for each sensor
        for (const [sensorId, readings] of Object.entries(data.readings)) {
            if (readings && readings.length > 0) {
                const totalReadings = readings.length;
                const verifiedReadings = readings.filter(r => r.IsVerified).length;
                const verificationPercentage = totalReadings > 0 ? 
                    Math.round((verifiedReadings / totalReadings) * 100) : 0;
                
	        // Generate a color based on verification percentage
                const hue = verificationPercentage * 1.2; // 0% = red (0), 100% = green (120)
                const bgColor = 'hsl(' + hue + ', 70%, 90%)';
                const textColor = 'hsl(' + hue + ', 70%, 30%)';
                
                // Create stat card
                const statCard = document.createElement('div');
                statCard.className = 'stat-card';
                statCard.style.backgroundColor = bgColor;
                statCard.style.borderLeft = '4px solid ' + textColor;
                
                const sensorLabel = document.createElement('div');
                sensorLabel.className = 'stat-label';
                sensorLabel.textContent = sensorId;
                
                const statValue = document.createElement('div');
                statValue.className = 'stat-value';
                statValue.textContent = verifiedReadings + ' / ' + totalReadings;
                statValue.style.color = textColor;
                
                const percentageLabel = document.createElement('div');
                percentageLabel.className = 'stat-percentage';
                percentageLabel.textContent = verificationPercentage + '% Verified';

                
                statCard.appendChild(sensorLabel);
                statCard.appendChild(statValue);
                statCard.appendChild(percentageLabel);
                
                statsContainer.appendChild(statCard);
            }
        }
    }
		
		function logout() {
			fetch('/api/logout', { method: 'POST' })
				.then(response => {
					if (response.ok) {
						alert(response.statusText);
					} else {
						console.error("Erreur lors de la demande de déconnexion");
						alert("Erreur lors de la déconnexion");
					}
				})
				.catch(error => {
					console.error("Erreur réseau:", error);
					alert("Erreur réseau lors de la déconnexion");
				});
		}
				

        // Fetch data from the API and update the UI
        function fetchData() {
            fetch('/api/data')
                .then(response => response.json())
                .then(data => {
                    updateUI(data);
		    updateChart(data);
                    updateVerificationStats(data); 
                })
                .catch(error => {
                    console.error('Error fetching data:', error);
                });
        }
	function takeSnapshot() {
	    fetch('/api/snapshot', { method: 'POST' })
		.then(response => {
		    if (!response.ok) throw new Error('Snapshot failed');
		    return response.text();
		})
		.then(msg => {
		     alert(msg); // success feedback
		})
		.catch(error => {
		    console.error('Snapshot error:', error);
		    alert('Failed to take snapshot');
		});
	}


        // Update the UI with the fetched data
        function updateUI(data) {
            const sensorsContainer = document.getElementById('sensors-data');
            sensorsContainer.innerHTML = '';
            
            document.getElementById('last-updated').textContent = 'Last updated: ' + new Date().toLocaleTimeString();
            
            // Create UI elements for each sensor
            for (const [sensorId, readings] of Object.entries(data.readings)) {
                const sensorElement = document.createElement('div');
                sensorElement.className = 'sensor-container';
                
                // Sensor header
                const sensorHeader = document.createElement('h2');
                sensorHeader.textContent = 'Sensor: ' + sensorId;
                sensorElement.appendChild(sensorHeader);
                
                // Readings
                if (readings && readings.length > 0) {
                    const readingsHeader = document.createElement('h3');
                    readingsHeader.textContent = 'Readings';
                    sensorElement.appendChild(readingsHeader);
                    
                    readings.forEach(function(reading) {
                        const readingElement = document.createElement('div');
                        readingElement.className = 'reading' + (reading.IsVerified ? ' verified' : '');
                        
                        const idDiv = document.createElement('div');
                        idDiv.textContent = 'ID: ' + (reading.ReadingID || 'Unknown');
                        readingElement.appendChild(idDiv);
                        
                        const tempDiv = document.createElement('div');
                        tempDiv.textContent = 'Temperature: ' + (reading.Temperature !== undefined ? 
                            parseFloat(reading.Temperature).toFixed(2) : 'N/A');
                        readingElement.appendChild(tempDiv);
                        
                        const clockDiv = document.createElement('div');
                        clockDiv.textContent = 'Clock: ' + (reading.Clock || 'N/A');
                        readingElement.appendChild(clockDiv);
                        
                        const verifiedDiv = document.createElement('div');
                        if (reading.IsVerified) {
                            verifiedDiv.textContent = 'Verified by: ' + (reading.VerifierID || 'Unknown');
                        } else {
                            verifiedDiv.textContent = 'Not verified yet';
                        }
                        readingElement.appendChild(verifiedDiv);
                        
                        sensorElement.appendChild(readingElement);
                    });
                } else {
                    const noReadings = document.createElement('p');
                    noReadings.textContent = 'No readings available';
                    sensorElement.appendChild(noReadings);
                }
                
                // Predictions
                if (data.predictions && data.predictions[sensorId] && data.predictions[sensorId].length > 0) {
                    const predictionElement = document.createElement('div');
                    predictionElement.className = 'prediction';
                    const latestPrediction = data.predictions[sensorId][data.predictions[sensorId].length - 1];
                    predictionElement.textContent = 'Latest prediction: ' + 
                        (latestPrediction !== undefined ? parseFloat(latestPrediction).toFixed(4) : 'N/A');
                    sensorElement.appendChild(predictionElement);
                }
                
                sensorsContainer.appendChild(sensorElement);
            }
        }

        // Initial data fetch
        fetchData();
        
        // Set up automatic refresh every 5 seconds
        setInterval(fetchData, 5000);
    </script>
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleAPIData serves the node's data in JSON format
func (u *UserNode) handleAPIData(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.isRunning {
		http.Error(w, "User node is not running", http.StatusInternalServerError)
		return
	}

	// Create a clean copy of readings to ensure proper JSON serialization
	readingsCopy := make(map[string][]models.Reading)
	for sensor, readings := range u.recentReadings {
		readingsCopy[sensor] = make([]models.Reading, len(readings))
		copy(readingsCopy[sensor], readings)
	}

	// Create a clean copy of predictions
	predictionsCopy := make(map[string][]float32)
	for sensor, predictions := range u.recentPredictions {
		predictionsCopy[sensor] = make([]float32, len(predictions))
		copy(predictionsCopy[sensor], predictions)
	}

	// Prepare data for JSON response
	data := struct {
		NodeName    string                      `json:"nodeName"`
		Readings    map[string][]models.Reading `json:"readings"`
		Predictions map[string][]float32        `json:"predictions"`
		Timestamp   int64                       `json:"timestamp"`
	}{
		NodeName:    u.GetName(),
		Readings:    readingsCopy,
		Predictions: predictionsCopy,
		Timestamp:   time.Now().Unix(),
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	// Encode and send JSON response
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("%s: Error encoding JSON: %v", u.GetName(), err)
		http.Error(w, "Error generating data", http.StatusInternalServerError)
		return
	}
}

func (n *UserNode) GetLocalState() string {
	n.mu.Lock()
	snap_content := ""
	for senderID, readings := range n.recentReadings {
		snap_content += senderID + ":"
		for _, reading := range readings {
			snap_content += reading.ReadingID + ","
		}
		snap_content += utils.PearD_SITE_SEPARATOR
	}
	n.mu.Unlock()
	return snap_content
}

func (u *UserNode) GetApplicationState() map[string][]models.Reading {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Deep copy of recentReadings
	readingsCopy := make(map[string][]models.Reading)
	for k, a := range u.recentReadings {
		readingsCopy[k] = make([]models.Reading, len(a))
		copy(readingsCopy[k], a)
	}

	return readingsCopy
}

func (u *UserNode) SetApplicationState(state map[string][]models.Reading) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.recentReadings = state
	format.Display_g(u.GetName(), "SetApplicationState", "Successfully restored recentReadings.")

	// Initialize recentPredictions to be empty. It will be populated as new readings are processed.
	u.recentPredictions = make(map[string][]float32)
	format.Display_g(u.GetName(), "SetApplicationState", "Initialized recentPredictions as empty.")
}

func (u *UserNode) Logout() {
	u.mu.Lock()
	if !u.isRunning {
		u.mu.Unlock()
		return
	}
	u.isRunning = false
	u.mu.Unlock()

	// Prévenir voisins via ControlLayer
	u.ctrlLayer.SendLogoutAnnouncement()
	u.ctrlLayer.SendConnectNeighbors()

	// Fermer canal vers ControlLayer (channel_to_ctrl)
	if u.channel_to_ctrl != nil {
		close(u.channel_to_ctrl)
	}

	// Notifier ControlLayer que User se déconnecte
	u.ctrlLayer.NotifyUserLogout()

	format.Display_g(u.GetName(), "Logout", "User node logged out and notified ControlLayer")
}

func (u *UserNode) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	format.Display_g(u.GetName(), "LogoutHandler", "LogoutHandler appelé via HTTP")
	go u.Logout()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Logout initiated"))
}
