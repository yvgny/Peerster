package main

import (
	"flag"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"os"
)

func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	msgArg := flag.String("msg", "", "message to be sent")
	// TODO est-ce permis de rajouter un argument au CLI ?

	flag.Parse()

	packet := &common.GossipPacket{
		Rumor: &common.RumorMessage{
			Text: *msgArg,
		},
	}

	err := common.SendMessage(common.LocalAddress+":"+*uiPortArg, packet, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
