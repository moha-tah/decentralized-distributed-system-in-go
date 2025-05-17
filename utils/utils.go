package utils

const VALUES_TO_STORE = 15 // per sensor
const DECAY_FACTOR = 0.9   // used in Users prediction
const PearD_SITE_SEPARATOR = "@" // used in pear_discovery msg to separate each site 
const PearD_VERIFIER_SEPARATOR = "|" // separates the list of ctrl names from the list of verifiers

func Synchronise(x int, y int) int {
	return max(x, y) + 1
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
