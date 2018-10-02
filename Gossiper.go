package main

import (
	"errors"
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/yvgny/Peerster/common"
	"net"
	"strings"
)

type Gossiper struct {
	clientAddress *net.UDPAddr
	gossipAddress *net.UDPAddr
	clientConn    *net.UDPConn
	gossipConn    *net.UDPConn
	name          string
	peers         *common.ConcurrentSet
	simple        bool
}

func NewGossiper(clientAddress, gossipAddress, name, peers string, simpleBroadcastMode bool) (*Gossiper, error) {
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

	if len(peers) > 0 {
		for _, addr := range strings.Split(peers, ",") {
			peersSet.Store(addr)
		}
	}

	return &Gossiper{
		gossipAddress: gAddress,
		clientAddress: cAddress,
		clientConn:    cConn,
		gossipConn:    gConn,
		name:          name,
		peers:         peersSet,
		simple:        simpleBroadcastMode,
	}, nil
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
				continue
			}

			if g.simple {
				peers := g.peers.Elements()
				fmt.Printf("CLIENT MESSAGE %s\n", gossipPacket.Simple.Contents)
				fmt.Println(strings.Join(peers, ","))

				gossipPacket.Simple.OriginalName = g.name
				errorList := common.BroadcastMessage(peers, gossipPacket, nil, g.gossipConn)
				if errorList != nil {
					for _, err := range errorList {
						fmt.Println(err.Error())
					}
				}
			}
		}
	}()

	// Handle peers messages
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, addr, err := g.gossipConn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			gossipPacket := &common.GossipPacket{}
			err = protobuf.Decode(buffer[0:n], gossipPacket)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			if g.simple {
				g.peers.Store(addr.String())
				peers := g.peers.Elements()
				fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", gossipPacket.Simple.OriginalName, addr,
					gossipPacket.Simple.Contents)
				fmt.Println(strings.Join(peers, ","))

				relayAddr := addr.String()
				errList := common.BroadcastMessage(peers, gossipPacket, &relayAddr, g.gossipConn)
				for _, err := range errList {
					fmt.Println(err.Error())
				}
			}
		}
	}()
}
