package models

import (
	"math"
)

// DecayedWeightedMean computes a decayed weighted mean of a slice of float64.
// decay should be between 0 and 1 (e.g., 0.9 means slower decay, 0.5 means faster decay).
func DecayedWeightedMean(values []float32, decay float32) float32 {
	if len(values) == 0 || decay <= 0 || decay >= 1 {
		return 0
	}

	var weightedSum float64
	var totalWeight float64
	n := len(values)

	for i := 0; i < n; i++ {
		weight := math.Pow(float64(decay), float64(n-1-i))
		weightedSum += float64(values[i]) * weight
		totalWeight += weight
	}

	return float32(weightedSum / totalWeight)
}

// LinearMean computes the default (linear) mean of a slice of float32.
func LinearMean(values []float32, _ float32) float32 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, value := range values {
		sum += float64(value)
	}

	return float32(sum / float64(len(values)))
}
