package node

import (
	// "fmt"
	"distributed_system/format"
	"distributed_system/models"
	"distributed_system/utils"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

// SensorNode represents a temperature sensor in the system
type SensorNode struct {
	BaseNode
	readInterval   time.Duration
	errorRate      float64
	recentReadings []float32
}

// NewSensorNode creates a new sensor node
func NewSensorNode(id string, interval time.Duration, errorRate float64) *SensorNode {
	return &SensorNode{
		BaseNode:       NewBaseNode(id, "sensor"),
		readInterval:   interval,
		errorRate:      errorRate,
		recentReadings: make([]float32, 0, 15),
	}
}

func (s *SensorNode) InitVectorClockWithSites(sites []string) {
	s.vectorClock = make([]int, len(sites))
	s.nodeIndex = utils.FindIndex(s.GetControlName(), sites)
}

// Start begins the sensor's operation
func (s *SensorNode) Start() error {
	format.Display(format.Format_d(
		"Start()", "node_sensor.go",
		"Starting sensor node "+s.GetName()))

	s.isRunning = true
	
	go func() {
		for {
			s.clock = s.clock + 1

			// ✅ Incrémenter l’horloge vectorielle locale
			s.vectorClock[s.nodeIndex] += 1

			// Générer une lecture
			reading := s.generateReading()

			// ✅ Stocker dans le FIFO local
			if len(s.recentReadings) >= 15 {
				s.recentReadings = s.recentReadings[1:]
			}
			s.recentReadings = append(s.recentReadings, float32(reading.Temperature))

			// Créer le message
			msg_id := s.GenerateUniqueMessageID()
			msg := format.Msg_format_multi(format.Build_msg_args(
				"id", msg_id,
				"type", "new_reading",
				"sender_name", s.GetName(),
				"sender_name_source", s.GetName(),
				"sender_type", s.Type(),
				"destination", "applications",
				"clk", strconv.Itoa(s.clock),
				"content_type", "sensor_reading",
				"content_value", strconv.FormatFloat(reading.Temperature, 'f', -1, 32),
				"vector_clock", utils.SerializeVectorClock(s.vectorClock),
			))

			s.logFullMessage(msg_id, reading)

			// Envoi vers couche application
			if s.ctrlLayer.SendApplicationMsg(msg) == nil {
				s.nbMsgSent = s.nbMsgSent + 1
			}

			time.Sleep(2 * time.Second)
		}
	}()
	select {} // Block forever
}

// generateReading produces a simulated temperature reading
func (s *SensorNode) generateReading() models.Reading {
	// Generate a base realistic temperature (here, between 15°C and 30°C)
	baseTemp := 15.0 + rand.Float64()*15.0

	// Sometimes introduce errors based on errorRate
	if rand.Float64() < s.errorRate {
		// Generate an erroneous reading (very high or very low)
		if rand.Float64() < 0.5 {
			// Abnormally high
			baseTemp = baseTemp + 50.0 + rand.Float64()*100.0
		} else {
			// Abnormally low
			baseTemp = baseTemp - 50.0 - rand.Float64()*100.0
		}
	}

	// Add some minor natural variation
	temperature := baseTemp + (rand.Float64()-0.5)*2.0

	return models.Reading{
		// ReadingID:   s.GenerateUniqueMessageID(),
		Temperature: temperature,
		Timestamp:   time.Now(),
		SensorID:    s.ID(),
		IsVerified:  false,
	}
}

func (s *SensorNode) initLogFile() {
	filename := "node_" + s.ID() + "_log.txt"
	f, err := os.Create(filename)
	if err == nil {
		defer f.Close()
		f.WriteString("# Log de node " + s.ID() + " créé à " + time.Now().Format(time.RFC3339) + "\n")
	}
}

// logSensorReading enregistre une lecture dans un fichier texte
// func (s *SensorNode) logSensorReading(temperature float32) {
// 	filename := "sensor_" + s.ID() + "_data.log"

// 	// Créer le format inspiré de message.go
// 	line := "/" + "timestamp=" + time.Now().Format(time.RFC3339) +
// 		"/sensor_id=" + s.ID() +
// 		"/temp=" + strconv.FormatFloat(float64(temperature), 'f', 2, 64) +
// 		"/state=" + "0" + "\n"

// 	// Ajouter au fichier
// 	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	if err == nil {
// 		defer f.Close()
// 		f.WriteString(line)
// 	}
// }

func (s *SensorNode) logFullMessage(msg_id string, reading models.Reading) {
	filename := "node_" + s.ID() + "_log.txt"

	destination := "applications" // Par défaut
	// Si tu veux tester le cas où il n'y a pas de destinataire, remplace par `destination := ""`

	logLine := "/" + "id=" + msg_id +
		"/type=new_reading" +
		"/sender_name=" + s.GetName() +
		"/sender_name_source=" + s.GetName() +
		"/sender_type=" + s.Type() +
		"/destination=" + destination +
		"/clk=" + strconv.Itoa(s.clock) +
		"/vector_clock=" + utils.SerializeVectorClock(s.vectorClock) +
		"/content_type=sensor_reading" +
		"/content_value=" + strconv.FormatFloat(reading.Temperature, 'f', -1, 32) +
		"\n"

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(logLine)
	}
}

func (s *SensorNode) ID() string   { return s.id }
func (s *SensorNode) Type() string { return s.nodeType }
func (s *SensorNode) HandleMessage(channel chan string) {
		for msg := range channel {
			// 🔎 Identifier le type de message
			msgType := format.Findval(msg, "type", s.GetName())

			// 🔁 Mettre à jour le vector clock à la réception
			vcStr := format.Findval(msg, "vector_clock", s.GetName())
			recvVC, err := utils.DeserializeVectorClock(vcStr)
			if err == nil {
				for i := 0; i < len(s.vectorClock); i++ {
					s.vectorClock[i] = utils.Max(s.vectorClock[i], recvVC[i])
				}
				s.vectorClock[s.nodeIndex] += 1
			}


			switch msgType {

			case "snapshot_request":
				format.Display("[SensorNode] ✅snapshot_request reçu")
				// 🔁 Mettre à jour l'horloge vectorielle à la réception du message
				vcStr := format.Findval(msg, "vector_clock", s.GetName())
				recvVC, err := utils.DeserializeVectorClock(vcStr)
				if err == nil {
					for i := 0; i < len(s.vectorClock); i++ {
						s.vectorClock[i] = utils.Max(s.vectorClock[i], recvVC[i])
					}
					s.vectorClock[s.nodeIndex] += 1
				}

				// 🧠 Lire les dernières valeurs stockées
				readings := make([]string, len(s.recentReadings))
				for i, val := range s.recentReadings {
					readings[i] = strconv.FormatFloat(float64(val), 'f', 2, 32)
				}
				readingsStr := "[" + strings.Join(readings, ", ") + "]"

				// 📨 Création du message snapshot_response
				originalRequester := format.Findval(msg, "sender_name", s.GetName())
				msgID := s.GenerateUniqueMessageID()
				msgResponse := format.Msg_format_multi(format.Build_msg_args(
					"id", msgID,
					"type", "snapshot_response",
					"sender_name", originalRequester,
					"sender_name_source", s.GetName(),
					"sender_type", s.Type(),
					"destination", format.Findval(msg, "sender_name_source", s.GetName()),
					"clk", strconv.Itoa(s.clock),
					"vector_clock", utils.SerializeVectorClock(s.vectorClock),
					"content_type", "snapshot_data",
					"content_value", readingsStr,
				))

				// 🗂️ Log optionnel
				format.Display(format.Format_d(s.GetName(), "HandleMessage()", "Sending snapshot_response: "+readingsStr))

				s.ctrlLayer.SendApplicationMsg(msgResponse)
				// Envoi vers couche application
				if s.ctrlLayer.SendApplicationMsg(msg) == nil {
					s.nbMsgSent = s.nbMsgSent + 1
				}
			}
		}
}
