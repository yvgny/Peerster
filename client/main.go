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
	destArg := flag.String("dest", "", "destination for the private message")

	flag.Parse()

	packet := &common.GossipPacket{}

	if *destArg != "" {
		packet.Private = &common.PrivateMessage{
			Destination:*destArg,
			Text:*msgArg,
		}
	} else {
		packet.Rumor = &common.RumorMessage{
			Text: *msgArg,
		}
	}

	err := common.SendMessage(common.LocalAddress+":"+*uiPortArg, packet, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
