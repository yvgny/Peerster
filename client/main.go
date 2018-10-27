package main

import (
	"flag"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"os"
	"path/filepath"
)

const SharedFilesFolder string = "_SharedFiles"

func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	msgArg := flag.String("msg", "", "message to be sent")
	destArg := flag.String("dest", "", "destination for the private message")
	fileArg := flag.String("file", "", "file to be indexed by the gossiper")

	flag.Parse()

	packet := &common.GossipPacket{}

	if *destArg != "" {
		packet.Private = &common.PrivateMessage{
			Destination: *destArg,
			Text:        *msgArg,
		}
	} else if *fileArg != "" {
		ex, err := os.Executable()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		exPath := filepath.Dir(ex)
		pathStr := filepath.Join(exPath, SharedFilesFolder, *fileArg)
		exists, _ := common.FileExists(pathStr)
		if !exists {
			fmt.Printf("File %s does not exists in folder %s\n", *fileArg, SharedFilesFolder)
			os.Exit(1)
		}
		packet.File = &common.FilePacket{
			Path: pathStr,
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
