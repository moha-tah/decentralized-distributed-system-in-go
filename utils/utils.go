package utils

const VALUES_TO_STORE = 15

func Synchronise(x int, y int) int {
	if x < y {
		return y + 1
	}
	return x + 1
}
