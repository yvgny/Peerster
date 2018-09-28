package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
)

const LocalAddress = "127.0.0.1"


func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	gossipAddressArg := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	nameArg := flag.String("name", "", "name of the gossiper")
	peersArg := flag.String("peers", "", "coma separated list of peers of the form ip:port")
	broadcastModeArg := flag.Bool("simple", false, "run gossiper in simple broadcast mode")
	flag.Parse()
	fmt.Printf("UIPort = %s, gossipAddr = %s, name = %s, peers = %s, broadcastEnabled = %t\n", *uiPortArg, *gossipAddressArg, *nameArg, *peersArg, *broadcastModeArg)

	gossiper, err := NewGossiper(LocalAddress+":"+*uiPortArg, *gossipAddressArg, *nameArg, *peersArg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	gossiper.handleClients()

	// Let the server run
	wg := &sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}
