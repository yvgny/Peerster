package main

import "sync"

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
	rt.table.Store(nextHop, nextHop)
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
