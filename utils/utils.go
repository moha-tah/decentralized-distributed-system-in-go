package utils

const VALUES_TO_STORE = 15 // per sensor

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
