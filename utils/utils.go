package utils

import (
	"fmt"
	"strconv"
	"strings"
)

const VALUES_TO_STORE = 15           // per sensor
const DECAY_FACTOR = 0.5             // used in Users prediction
const PearD_SITE_SEPARATOR = "@"     // used in pear_discovery msg to separate each site
const PearD_VERIFIER_SEPARATOR = "|" // separates the list of ctrl names from the list of verifiers

func Synchronise(x int, y int) int {
	return max(x, y) + 1
}

func SynchroniseVectorClock(vc1 []int, vc2 []int, caller_index int) ([]int, error) {
	if len(vc1) != len(vc2) {
		return nil, fmt.Errorf(fmt.Sprintf("utils.SynchroniseVectorClock: vector clocks must be of the same length. Got %d and %d", len(vc1), len(vc2)))
	}
	if len(vc1) == 0 {
		return vc2, nil
	} else if len(vc2) == 0 {
		return vc1, nil
	}
	newVC := make([]int, len(vc1))
	for i := 0; i < len(vc1); i++ {
		newVC[i] = max(vc1[i], vc2[i])
	}
	newVC[caller_index] += 1
	return newVC, nil
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

func ExtractIDFromName(name string) (int, error) {
	// format is "type (id)"
	idStr_parts := strings.Split(name, "(")
	if len(idStr_parts) != 2 {
		return -1, fmt.Errorf("error parsing id from %q: expected format 'type (id)'", name)
	}
	idStr := idStr_parts[1]
	idStr = strings.TrimSuffix(idStr, ")")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return -1, fmt.Errorf("error parsing id from %q: %v", name, err)
	}
	return id, nil
}

// =============== ELECTION ===============
// Extract the id from the node names and return
// the minimum id
func UpdateLeader(name1, name2 string) (string, error) {
	if name1 == "" && name2 == "" {
		return "", fmt.Errorf("utils.updateLeader: both names are empty")
	} else if name1 == "" {
		return name2, nil
	} else if name2 == "" {
		return name1, nil
	}

	// Retrieve ID from the sender name
	id1, err := ExtractIDFromName(name1)
	if err != nil {
		panic(fmt.Sprintf("utils.updateLeader: error parsing id1 from %q: %v", name1, err))
	}

	id2, err := ExtractIDFromName(name2)
	if err != nil {
		panic(fmt.Sprintf("utils.updateLeader: error parsing id2 from %q: %v", name2, err))
	}
	if id1 < id2 {
		return name1, nil
	}
	return name2, nil

}
