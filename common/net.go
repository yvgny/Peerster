package common

import (
	"errors"
	"github.com/dedis/protobuf"
	"net"
)

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

type GossipPacket struct {
	Simple *SimpleMessage
}

func SendMessage(address string, packet *GossipPacket) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return errors.New("Cannot resolve gossiper address: " + err.Error())
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return errors.New("Cannot connect to gossiper: " + err.Error())
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

func BroadcastMessage(hosts []string, message *GossipPacket, sender string) error {
	for _, host := range hosts {
		if host == sender {
			continue
		}
		err := SendMessage(host, message)
		if err != nil {
			return err
		}
	}

	return nil
}
