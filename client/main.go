package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/yvgny/Peerster/common"
	"net"
	"os"
)

const LocalAddress = "127.0.0.1"

func main() {
	uiPortArg := flag.String("UIPort", "8080", "port for the UI client")
	msgArg := flag.String("msg", "", "message to be sent")
	flag.Parse()

	err := sendMessage(LocalAddress+":"+*uiPortArg, *msgArg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func sendMessage(address, message string) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return errors.New("Cannot resolve gossiper address: " + err.Error())
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return errors.New("Cannot connect to gossiper: " + err.Error())
	}

	packet := &common.GossipPacket{
		Simple: &common.SimpleMessage{
			Contents: message,
		},
	}

	packetByte, err := protobuf.Encode(packet)
	if err != nil {
		return errors.New("Cannot encode packet: " + err.Error())
	}

	_, err = conn.Write(packetByte)
	if err != nil {
		return errors.New("Cannot send message: " + err.Error())
	}

	return nil
}
