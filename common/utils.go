package common

import (
	"math/rand"
	"os"
)

// Returns true with probability 1/2
func FlipACoin() bool {
	return rand.Int()%2 == 0
}

func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func Unique(array []string) []string {
	keys := make(map[string]bool)
	list := make([]string, 0)
	for _, entry := range array {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}

	return list
}

func Contains(elem string, slice []string) bool {
	for _, e := range slice {
		if e == elem {
			return true
		}
	}

	return false
}
