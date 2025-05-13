package node

import (
	// "fmt"
	"time"
	"strconv"
	"math/rand"
	"distributed_system/format"
	"distributed_system/models"
)

// SensorNode represents a temperature sensor in the system
type SensorNode struct {
	BaseNode
	readInterval time.Duration
	errorRate    float64
}

// NewSensorNode creates a new sensor node
func NewSensorNode(id string, interval time.Duration, errorRate float64) *SensorNode {
    return &SensorNode{
        BaseNode:     NewBaseNode(id, "sensor"),
        readInterval: interval,
        errorRate:    errorRate,
    }
}

// Start begins the sensor's operation
func (s *SensorNode) Start() error {
	format.Display(format.Format_d(
		"Start()",
		"node_sensor.go",
		"Starting sensor node " + s.GetName()))

	s.isRunning = true
    
    	for {
		s.clock = s.clock + 1

        	// Generate a reading
            	reading := s.generateReading()
            

		var msg_id string = s.GenerateUniqueMessageID()
		msg := format.Msg_format_multi(
			format.Build_msg_args(
				"id", msg_id,
				"type", "new_reading",
				"sender_name", s.GetName(),
				"sender_name_source", s.GetName(),
				"sender_type", s.Type(),
				"destination", "applications",
				"clk", strconv.Itoa(s.clock),
				"content_type", "sensor_reading",
				"content_value", strconv.FormatFloat(reading.Temperature, 'f', -1, 32)))

		if s.ctrlLayer.SendApplicationMsg(msg) == nil { // no error => message has been sent
			s.nbMsgSent = s.nbMsgSent + 1
		} 

		time.Sleep(time.Duration(2) * time.Second)
            
	}
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
    temperature := baseTemp + (rand.Float64() - 0.5) * 2.0
    
    return models.Reading{
        // ReadingID:   s.GenerateUniqueMessageID(),
        Temperature: temperature,
        Timestamp:   time.Now(),
        SensorID:    s.ID(),
        IsVerified:  false,
    }
}

func (s *SensorNode) ID() string          { return s.id }
func (s *SensorNode) Type() string        { return s.nodeType }
func (s *SensorNode) HandleMessage(channel chan string) {
    // Nothing needed. Placeholder for full implementation of interface Node
}
