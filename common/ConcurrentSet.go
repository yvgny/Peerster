package common

import (
	"strings"
	"sync"
)

type ConcurrentSet struct {
	sync.RWMutex
	internalMap map[string]bool
}

func NewConcurrentSet() *ConcurrentSet {
	return &ConcurrentSet{
		internalMap: make(map[string]bool),
	}
}

func (cs *ConcurrentSet) Store(key string) {
	cs.RLock()
	defer cs.RUnlock()
	cs.internalMap[key] = true
}

func (cs *ConcurrentSet) Delete(key string) {
	cs.Lock()
	defer cs.Unlock()
	delete(cs.internalMap, key)
}

func (cs *ConcurrentSet) Exists(key string) bool {
	cs.RLock()
	defer cs.RUnlock()
	return cs.internalMap[key]
}

func (cs *ConcurrentSet) Join(separator string) string {
	cs.RLock()
	defer cs.RUnlock()
	result := ""
	for key := range cs.internalMap {
		result += key + separator
	}

	return strings.TrimSuffix(result, separator)
}

func (cs *ConcurrentSet) Elements() []string {
	peers := make([]string, 0, len(cs.internalMap))
	cs.RLock()
	defer cs.RUnlock()
	for key := range cs.internalMap {
		peers = append(peers, key)
	}
	return peers
}
