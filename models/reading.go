package models

import (
    "fmt"
    "time"
)

// Reading represents a temperature reading from a sensor
type Reading struct {
    ReadingID        string    `json:"reading_id"`
    Temperature      float32   `json:"temperature"`
    Timestamp        time.Time `json:"timestamp"`
	Clock	     int       // prefer clock instead of timestamp
    SensorID         string    `json:"sensor_id"`
    Location         string    `json:"location"`
    IsVerified       bool      `json:"is_verified"`
    VerifierID       string    `json:"verifier_id,omitempty"`
    VerificationTime time.Time `json:"verification_time,omitempty"`
}

// GetDayID returns a string identifier for the day of this reading
func (r Reading) GetDayID() string {
    return GetDayIDFromTime(r.Timestamp)
}

// GetDayIDFromTime generates a day identifier from a timestamp
func GetDayIDFromTime(t time.Time) string {
    return t.Format("2006-01-02")
}

// WeatherPrediction represents a forecast for future temperature
type WeatherPrediction struct {
    PredictedTemperature float32   `json:"predicted_temperature"`
    PredictionTime       time.Time `json:"prediction_time"`
    Confidence           float32   `json:"confidence"`
    VerifiedReadings     int       `json:"verified_readings"`
    UnverifiedReadings   int       `json:"unverified_readings"`
    TotalReadings        int       `json:"total_readings"`
}

// String returns a formatted string representation of the prediction
func (wp WeatherPrediction) String() string {
    return fmt.Sprintf(
        "Weather prediction for %s: %.2f°C (confidence: %.2f%%)",
        wp.PredictionTime.Format("2006-01-02"),
        wp.PredictedTemperature,
        wp.Confidence * 100,
    )
}

// FlattenReadings takes a map of ID to slices-of-readings and flattens 
// it into a single slice of readings. This is useful for processing or displaying
// all readings together.
func FlattenReadings(readingsMap map[string][]Reading) []Reading {
    // First, calculate the total number of readings
    total := 0
    for _, readings := range readingsMap {
        total += len(readings)
    }

    // Preallocate the slice
    flattened := make([]Reading, 0, total)

    // Append all readings
    for _, readings := range readingsMap {
        flattened = append(flattened, readings...)
    }

    return flattened
}

