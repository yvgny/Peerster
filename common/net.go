package common

import (
	"errors"
	"github.com/dedis/protobuf"
	"net"
)

const LocalAddress = "127.0.0.1"

type SimpleMessage struct {
	OriginalName string
	Contents     string
}

type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}

type PeerStatus struct {
	Identifier string
	NextID     uint32
}

type StatusPacket struct {
	Want []PeerStatus
}

type GossipPacket struct {
	Simple *SimpleMessage
	Rumor  *RumorMessage
	Status *StatusPacket
}

// Sends a GossipPacket at a specific host. A connection can be specified or can be nil.
// If it is nil, a new connection is openend on a random port. If a connection is given,
// it has to be unconnected. Thus, if the connection is opened using Dial, SendMessage
// will throw an error
func SendMessage(address string, packet *GossipPacket, conn *net.UDPConn) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return errors.New("Cannot resolve peer address: " + err.Error())
	}

	if conn == nil {
		conn, err = net.ListenUDP("udp", nil)
		if err != nil {
			return errors.New("Cannot open new connection: " + err.Error())
		}
	}

	packetByte, err := protobuf.Encode(packet)
	if err != nil {
		return errors.New("Cannot encode packet: " + err.Error())
	}

	_, err = conn.WriteToUDP(packetByte, udpAddr)
	if err != nil {
		return errors.New("Cannot send message: " + err.Error())
	}

	return nil
}

// Broadcast a message to a list of hosts. This uses SendMessage, so the connection can be nil
// (please refer to the function SendMessage for more information). Also, an optional sender
// can be specified. If it's the case, the message will be broadcast to every hosts except the sender
func BroadcastMessage(hosts []string, message *GossipPacket, sender *string, conn *net.UDPConn) []error {
	errorList := make([]error, 0)
	for _, host := range hosts {
		if sender != nil && host == *sender {
			continue
		}
		err := SendMessage(host, message, conn)
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	return errorList
}
