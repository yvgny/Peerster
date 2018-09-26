package common

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

type GossipPacket struct {
	Simple *SimpleMessage
}
