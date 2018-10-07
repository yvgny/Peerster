package common

import "math/rand"

func FlipACoin() bool {
	return rand.Int() % 2 == 0
}
