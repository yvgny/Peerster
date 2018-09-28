package main

import (
	"flag"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"os"
)

const LocalAddress = "127.0.0.1"

func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	msgArg := flag.String("msg", "", "message to be sent")
	flag.Parse()

	err := common.SendMessage(LocalAddress+":"+*uiPortArg, *msgArg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}