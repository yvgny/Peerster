package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	msgArg := flag.String("msg", "", "message to be sent")
	destArg := flag.String("dest", "", "destination for the private message")
	fileArg := flag.String("file", "", "file to be indexed by the gossiper, or filename of the requested file")
	requestArg := flag.String("request", "", "request a chunk or metafile of this hash")
	keywordsArg := flag.String("keywords", "", "search a file by keywords")
	budgetArg := flag.Int("budget", 0, "budget allowed for the expanding-ring search.")
	flag.Parse()

	packet := &common.ClientPacket{}

	privateMsg := *msgArg != "" && *destArg != "" && *fileArg == "" && *requestArg == "" && *keywordsArg == ""
	fileUpload := *msgArg == "" && *destArg == "" && *fileArg != "" && *requestArg == "" && *keywordsArg == ""
	rumorMsg := *msgArg != "" && *destArg == "" && *fileArg == "" && *requestArg == "" && *keywordsArg == ""
	fileRequestMsg := *msgArg == "" && *fileArg != "" && *requestArg != "" && *keywordsArg == ""
	fileSearch := *msgArg == "" && *destArg == "" && *fileArg == "" && *requestArg == "" && *keywordsArg != ""

	if privateMsg {
		packet.Private = &common.PrivateMessage{
			Destination: *destArg,
			Text:        *msgArg,
		}
	} else if fileUpload {
		pathStr := filepath.Join(common.SharedFilesFolder, *fileArg)
		exists, _ := common.FileExists(pathStr)
		if !exists {
			fmt.Printf("File %s does not exists in folder %s\n", *fileArg, common.SharedFilesFolder)
			os.Exit(1)
		}
		packet.FileIndex = &common.FileIndexPacket{
			Filename: *fileArg,
		}
	} else if rumorMsg {
		packet.Rumor = &common.RumorMessage{
			Text: *msgArg,
		}
	} else if fileRequestMsg {
		hash, err := hex.DecodeString(*requestArg)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		packet.FileDownload = &common.FileDownloadPacket{
			User:      *destArg,
			HashValue: hash,
			Filename:  *fileArg,
		}
	} else if fileSearch {
		if !(*budgetArg >= 0) {
			fmt.Println("Error: budget should be >= 0")
			os.Exit(1)
		}
		keywords := strings.Split(*keywordsArg, ",")
		searchRequest := common.SearchRequest{
			Budget:   uint64(*budgetArg),
			Keywords: keywords,
		}
		packet.SearchRequest = &searchRequest
	} else {
		fmt.Println("Error: combination of given arguments doesn't corresponds to any action")
		os.Exit(1)
	}

	err := common.SendMessage(common.LocalAddress+":"+*uiPortArg, packet, nil)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

}
