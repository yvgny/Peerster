package main

import (
	"fmt"
	"sync"
)

type RoutingTable struct {
	table *sync.Map
}

func NewRoutingTable() *RoutingTable {
	rt := RoutingTable{
		table: &sync.Map{},
	}

	return &rt
}

func (rt *RoutingTable) updateRoute(destination, nextHop string) {
	rt.table.Store(destination, nextHop)
}

func (rt *RoutingTable) getNextHop(destination string) (string, bool) {
	val, ok := rt.table.Load(destination)
	if !ok {
		return "", ok
	}
	nextHop, ok := val.(string)
	if !ok {
		return "", ok
	}

	return nextHop, ok
}

func (rt *RoutingTable) String() string {
	output := ""
	rt.table.Range(func(dest, host interface{}) bool {
		output += fmt.Sprintf("%s via %s\n", dest.(string), host.(string))
		return true
	})

	return output
}
