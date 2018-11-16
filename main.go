package main

import (
	"flag"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"github.com/yvgny/Peerster/gossiper"
	"os"
	"sync"
)


func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	gossipAddressArg := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	nameArg := flag.String("name", "", "name of the gossiper")
	peersArg := flag.String("peers", "", "coma separated list of peers of the form ip:port")
	broadcastModeArg := flag.Bool("simple", false, "run gossiper in simple broadcast mode")
	wsArg := flag.Bool("ws", false, "start a web server on 127.0.0.1:8080 with basic GUI")
	rtimer := flag.Int("rtimer", 0, "route rumors sending period in seconds, 0 to disable sending of route rumors")
	flag.Parse()
	fmt.Printf("UIPort = %s, gossipAddr = %s, name = %s, peers = %s, broadcastEnabled = %t\n", *uiPortArg, *gossipAddressArg, *nameArg, *peersArg, *broadcastModeArg)

	goss, err := gossiper.NewGossiper(common.LocalAddress+":"+*uiPortArg, *gossipAddressArg, *nameArg, *peersArg, *broadcastModeArg, *rtimer)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	var ws *gossiper.WebServer

	if *wsArg {
		ws = gossiper.NewWebServer(goss)
		ws.StartWebServer()
	}

	goss.StartGossiper()

	// Let the server run
	wg := &sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}
