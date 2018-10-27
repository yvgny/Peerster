package common

import (
	"math/rand"
	"os"
)

// Returns true with probability 1/2y
func FlipACoin() bool {
	return rand.Int()%2 == 0
}

func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil { return true, nil }
	if os.IsNotExist(err) { return false, nil }
	return true, err
}