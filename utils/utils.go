package utils

import (
	"fmt"
	"strconv"
	"strings"
)

const VALUES_TO_STORE = 15 // per sensor
const DECAY_FACTOR = 0.5   // used in Users prediction
const PearD_SITE_SEPARATOR = "@" // used in pear_discovery msg to separate each site 
const PearD_VERIFIER_SEPARATOR = "|" // separates the list of ctrl names from the list of verifiers


func Synchronise(x int, y int) int {
	return max(x, y) + 1
}

func SynchroniseVectorClock(vc1 []int, vc2 []int, caller_index int) []int {
	if len(vc1) != len(vc2) {
		panic("utils.SynchroniseVectorClock: vector clocks must be of the same length")
	}
	for i := 0; i < len(vc1); i++ {
		vc1[i] = max(vc1[i], vc2[i])
	}
	vc1[caller_index] += 1
	return vc1
}

func RemoveAllOccurrencesInt(slice []int, value int) []int {
	var result []int
	for _, v := range slice {
		if v != value {
			result = append(result, v)
		}
	}
	return result
}
func RemoveAllOccurrencesString(slice []string, value string) []string {
	var result []string
	for _, v := range slice {
		if v != value {
			result = append(result, v)
		}
	}
	return result
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func SerializeVectorClock(vc []int) string {
	parts := make([]string, len(vc))
	for i, val := range vc {
		parts[i] = strconv.Itoa(val)
	}
	return strings.Join(parts, ",")
}

func DeserializeVectorClock(vcStr string) ([]int, error) {
	parts := strings.Split(vcStr, ",")
	vc := make([]int, len(parts))
	for i, s := range parts {
		val, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		vc[i] = val
	}
	return vc, nil
}

func FindIndex(name string, sites []string) int {
	for i, site := range sites {
		if site == name {
			return i
		}
	}
	panic(fmt.Sprintf("utils.FindIndex: name %q not found in site list %v", name, sites))
}

func VectorClockCompatible(vc1, vc2 []int) bool {
	lessOrEqual := true
	greaterOrEqual := true
	for i := 0; i < len(vc1); i++ {
		if vc1[i] > vc2[i] {
			lessOrEqual = false
		}
		if vc2[i] > vc1[i] {
			greaterOrEqual = false
		}
	}
	return lessOrEqual || greaterOrEqual
}

func ParseFloatArray(s string) []float32 {
	parts := strings.Split(s, ",")
	var result []float32
	for _, part := range parts {
		if f, err := strconv.ParseFloat(strings.TrimSpace(part), 32); err == nil {
			result = append(result, float32(f))
		}
	}
	return result
}

func SliceIntEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SliceIntGreaterThan checks if all elements of a are smaller than b
// return true if a < b
func SliceIntLessThan(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] >= b[i] {
			return false
		}
	}
	return true
}

func Float32SliceToStringSlice(floats []float32) []string {
	strs := make([]string, len(floats))
	for i, f := range floats {
		strs[i] = fmt.Sprintf("%.5f", f) // format précis à 5 décimales
	}
	return strs
}
