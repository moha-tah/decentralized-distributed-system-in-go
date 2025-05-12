package utils

const VALUES_TO_STORE = 2

func Synchronise(x int, y int) int {
	return max(x, y) + 1
}
