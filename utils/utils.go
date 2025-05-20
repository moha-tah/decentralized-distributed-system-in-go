package utils

import (
	"fmt"
	"strconv"
	"strings"
)

const VALUES_TO_STORE = 15

func Synchronise(x int, y int) int {
	return max(x, y) + 1
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
