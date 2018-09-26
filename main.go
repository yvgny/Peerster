package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/yvgny/Peerster/common"
	"net"
	"os"
	"strings"
)

const LocalAddress = "127.0.0.1"

type Gossiper struct {
	clientAddress *net.UDPAddr
	gossipAddress *net.UDPAddr
	clientConn    *net.UDPConn
	gossipConn    *net.UDPConn
	name          string
	peers         *common.ConcurrentSet
}

func (g *Gossiper) handleClients() {
	// Handle clients messages
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, _, err := g.clientConn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			gossipPacket := &common.GossipPacket{}
			err = protobuf.Decode(buffer[0:n], gossipPacket)
			if err != nil {
				fmt.Println(err.Error())
			}

			peers := g.peers.Elements()
			fmt.Printf("CLIENT MESSAGE %s\n", gossipPacket.Simple.Contents)
			fmt.Println(strings.Join(peers, ","))
		}
	}()
	// Handle peers messages
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, _, err := g.gossipConn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			gossipPacket := &common.GossipPacket{}
			err = protobuf.Decode(buffer[0:n], gossipPacket)
			if err != nil {
				fmt.Println(err.Error())
			}

			peers := g.peers.Elements()
			fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", gossipPacket.Simple.OriginalName, gossipPacket.Simple.RelayPeerAddr,
				gossipPacket.Simple.Contents)
			fmt.Println(strings.Join(peers, ","))
		}
	}()
}

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
	for{}
}

func NewGossiper(clientAddress, gossipAddress, name, peers string) (*Gossiper, error) {
	cAddress, err := net.ResolveUDPAddr("udp4", clientAddress)
	if err != nil {
		return nil, errors.New("Cannot resolve client address " + clientAddress + ": " + err.Error())
	}
	gAddress, err := net.ResolveUDPAddr("udp4", gossipAddress)
	if err != nil {
		return nil, errors.New("Cannot resolve gossiper address " + gossipAddress + ": " + err.Error())
	}
	cConn, err := net.ListenUDP("udp4", cAddress)
	if err != nil {
		return nil, errors.New("Cannot open client connection: " + err.Error())
	}
	gConn, err := net.ListenUDP("udp4", gAddress)
	if err != nil {
		cConn.Close()
		return nil, errors.New("Cannot open gossiper connection: " + err.Error())
	}

	peersSet := common.NewConcurrentSet()

	for _, addr := range strings.Split(peers, ",") {
		peersSet.Store(addr)
	}

	return &Gossiper{
		gossipAddress: gAddress,
		clientAddress: cAddress,
		clientConn:    cConn,
		gossipConn:    gConn,
		name:          name,
		peers:         peersSet,
	}, nil
}
