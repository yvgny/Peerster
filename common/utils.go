package common

import "math/rand"

// Returns true with probability 1/2y
func FlipACoin() bool {
	return rand.Int()%2 == 0
}
