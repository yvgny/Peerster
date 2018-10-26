package main

import (
	"errors"
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/yvgny/Peerster/common"
	"net"
	"strings"
	"sync"
	"time"
)

type Gossiper struct {
	clientAddress *net.UDPAddr
	gossipAddress *net.UDPAddr
	clientConn    *net.UDPConn
	gossipConn    *net.UDPConn
	name          string
	peers         *common.ConcurrentSet
	clocks        *sync.Map
	waitAck       *sync.Map
	messages      *sync.Map
	routingTable  *RoutingTable
	simple        bool
	mutex         sync.Mutex
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
	clocksMap := sync.Map{}
	syncMap := sync.Map{}
	messagesMap := sync.Map{}

	if len(peers) > 0 {
		for _, addr := range strings.Split(peers, ",") {
			peersSet.Store(addr)
		}
	}

	g := &Gossiper{
		gossipAddress: gAddress,
		clientAddress: cAddress,
		clientConn:    cConn,
		gossipConn:    gConn,
		name:          name,
		peers:         peersSet,
		clocks:        &clocksMap,
		waitAck:       &syncMap,
		messages:      &messagesMap,
		routingTable:  NewRoutingTable(),
		simple:        simpleBroadcastMode,
	}

	g.startAntiEntropy(time.Second)

	return g, nil
}

// Start two listeners : one for the client side (listening on UIPort) and one
// for the peers (listening on gossipAddr)
func (g *Gossiper) StartGossiper() {
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

			// Handle the packet in a new thread to be able to listen on new messages again
			go func() {
				if gossipPacket.Rumor != nil {
					if err = g.HandleClientMessage(gossipPacket); err != nil {
						fmt.Println(err.Error())
					}
				} else if gossipPacket.Simple != nil && g.simple {
					output := fmt.Sprintf("CLIENT MESSAGE %s\n", gossipPacket.Simple.Contents)
					fmt.Println(output + g.peersString())
					gossipPacket.Simple.OriginalName = g.name
					errorList := common.BroadcastMessage(g.peers.Elements(), gossipPacket, nil, g.gossipConn)
					if errorList != nil {
						for _, err = range errorList {
							fmt.Println(err.Error())
						}
					}
				}
			}()
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

			// Handle the packet in a new thread to be able to listen on new messages again
			go func() {
				g.peers.Store(addr.String())

				if gossipPacket.Rumor != nil {
					output := fmt.Sprintf("RUMOR origin %s from %s ID %d contents %s\n", gossipPacket.Rumor.Origin, addr, gossipPacket.Rumor.ID, gossipPacket.Rumor.Text)
					fmt.Println(output + g.peersString())

					if g.isNewValidMessage(gossipPacket.Rumor) {
						g.clocks.Store(gossipPacket.Rumor.Origin, gossipPacket.Rumor.ID+1)
						g.storeMessage(gossipPacket.Rumor)

						// Update DSDV table
						g.routingTable.updateRoute(gossipPacket.Rumor.Origin, addr.String())
						fmt.Printf("DSDV %s %s", gossipPacket.Rumor.Origin, addr.String())

						// Send ack
						if err = g.sendStatusPacket(addr.String()); err != nil {
							fmt.Println(err.Error())
						}

						if err = g.startMongering(gossipPacket, nil, nil); err != nil {
							fmt.Println(err.Error())
						}
					} else {
						// Message ID was probably too high : trying to update
						if err = g.sendStatusPacket(addr.String()); err != nil {
							fmt.Println(err.Error())
						}
					}
				} else if gossipPacket.Status != nil {
					output := fmt.Sprintf("STATUS from %s", addr)
					ackWaiting := false
					addrString := addr.String()
					var ackChan *chan *common.StatusPacket
					for _, stat := range gossipPacket.Status.Want {
						// find if this is an acknowledgment
						value, ok := g.waitAck.Load(generateRumorUniqueString(&addrString, stat.Identifier, stat.NextID))
						if ok {
							ackWaiting = true
							ackChan = value.(*chan *common.StatusPacket)
						}
						// compute string
						output += " peer " + stat.Identifier + " nextID " + fmt.Sprint(stat.NextID)
					}
					fmt.Println(output + "\n" + g.peersString())

					if ackWaiting {
						*ackChan <- gossipPacket.Status
					} else {
						_, err = g.syncWithPeer(addr.String(), gossipPacket.Status)
						if err != nil {
							fmt.Println(err.Error())
						}
					}
				} else if gossipPacket.Simple != nil && g.simple {
					output := fmt.Sprintf("SIMPLE MESSAGE origin %s from %s contents %s\n", gossipPacket.Simple.OriginalName, addr,
						gossipPacket.Simple.Contents)
					fmt.Println(output + g.peersString())
					relayAddr := addr.String()
					errList := common.BroadcastMessage(g.peers.Elements(), gossipPacket, &relayAddr, g.gossipConn)
					for _, err = range errList {
						fmt.Println(err.Error())
					}
				}

			}()
		}
	}()
}

// Returns the current value of the clock of a peer and increment it
func (g *Gossiper) incrementClock(peer string) uint32 {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	currentClockInter, _ := g.clocks.LoadOrStore(g.name, uint32(1))
	currentClock := currentClockInter.(uint32)
	g.clocks.Store(g.name, currentClock+1)

	return currentClock
}

func (g *Gossiper) HandleClientMessage(packet *common.GossipPacket) error {
	packet.Rumor.Origin = g.name
	currentClock := g.incrementClock(g.name)
	output := fmt.Sprintf("CLIENT MESSAGE %s\n", packet.Rumor.Text)
	fmt.Println(output + g.peersString())
	packet.Rumor.ID = currentClock
	g.storeMessage(packet.Rumor)

	if err := g.startMongering(packet, nil, nil); err != nil {
		return err
	}

	return nil
}

func (g *Gossiper) AddPeer(peer string) error {
	addr, err := net.ResolveUDPAddr("udp4", peer)
	if err != nil {
		return err
	}

	g.peers.Store(addr.String())
	return nil
}

func (g *Gossiper) startAntiEntropy(period time.Duration) {
	go func() {
		ticker := time.NewTicker(period)
		for range ticker.C {
			randomHost, found := g.peers.Pick()
			if !found {
				continue
			} else if err := g.sendStatusPacket(randomHost); err != nil {
				fmt.Println(err.Error())
			}
		}
	}()
}

func (g *Gossiper) startMongering(gossipPacket *common.GossipPacket, host *string, lastHost *string) error {
	var toHost string
	var lastHostVal string
	if lastHost != nil {
		lastHostVal = *lastHost
	} else {
		lastHostVal = ""
	}
	if host != nil {
		toHost = *host
	} else {
		toHost = lastHostVal
		var found bool
		for i := 0; i < 30 && toHost == lastHostVal; i += 1 {
			toHost, found = g.peers.Pick()
			if !found {
				toHost = lastHostVal
			}
			i += 1
		}
		if toHost == lastHostVal {
			return errors.New("unable to find a new peer to monger with (last host is " + lastHostVal + ")")
		}
	}
	if lastHost != nil {
		fmt.Printf("FLIPPED COIN sending rumor to %s\n", toHost)
	}
	fmt.Printf("MONGERING with %s\n", toHost)
	err := common.SendMessage(toHost, gossipPacket, g.gossipConn)
	if err != nil {
		return err
	}
	g.waitForAck(toHost, gossipPacket, time.Second)
	return nil
}

// this will wait that the peer acknowledge this specific packet (ID + origin must be correct)
func (g *Gossiper) waitForAck(fromAddr string, forMsg *common.GossipPacket, timeout time.Duration) {
	ackChan := make(chan *common.StatusPacket)

	// we wait for the status that acks this Message (so wanted ID will be this ID + 1)
	UID := generateRumorUniqueString(&fromAddr, forMsg.Rumor.Origin, forMsg.Rumor.ID+1)
	g.waitAck.Store(UID, &ackChan)
	go func() {
		timer := time.NewTimer(timeout)
		select {
		case status := <-ackChan:
			g.waitAck.Delete(UID)

			didSync, err := g.syncWithPeer(fromAddr, status)
			if err != nil {
				fmt.Println(err.Error())
			} else if didSync {
				return
			}
		case <-timer.C:
			g.waitAck.Delete(UID)
		}

		if common.FlipACoin() {
			err := g.startMongering(forMsg, nil, &fromAddr)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}()
}

// Update peer using the status packet he sent. If an update was/is necessary, it will return true
func (g *Gossiper) syncWithPeer(peer string, status *common.StatusPacket) (bool, error) {
	for _, peerStatus := range status.Want {
		// Check if peer is up to date, if not send him the new messages
		if clock, ok := g.clocks.Load(peerStatus.Identifier); ok && clock.(uint32) > peerStatus.NextID {
			msg, ok := g.getMessage(peerStatus.Identifier, peerStatus.NextID)
			if !ok {
				return true, errors.New(fmt.Sprintf("message n°%d from %s is not stored but associated clock is %d", peerStatus.NextID+1, peerStatus.Identifier, clock))
			}
			gossipPack := common.GossipPacket{
				Rumor: msg,
			}
			if err := g.startMongering(&gossipPack, &peer, nil); err != nil {
				return true, err
			}

			return true, nil
		}
	}

	// Check if we are up to date by sending our current clocks status
	for _, peerStatus := range status.Want {
		if clock, _ := g.clocks.LoadOrStore(peerStatus.Identifier, uint32(1)); clock.(uint32) < peerStatus.NextID {
			err := g.sendStatusPacket(peer)
			return true, err
		}
	}

	fmt.Printf("IN SYNC WITH %s\n", peer)

	return false, nil
}

func (g *Gossiper) sendStatusPacket(peer string) error {
	statuses := make([]common.PeerStatus, 0)
	g.clocks.Range(func(key, value interface{}) bool {
		statuses = append(statuses, common.PeerStatus{
			Identifier: key.(string),
			NextID:     value.(uint32),
		})
		return true
	})

	packet := &common.GossipPacket{
		Status: &common.StatusPacket{
			Want: statuses,
		},
	}

	if err := common.SendMessage(peer, packet, g.gossipConn); err != nil {
		return err
	}

	return nil
}

func (g *Gossiper) storeMessage(message *common.RumorMessage) {
	g.messages.Store(generateRumorUniqueString(nil, message.Origin, message.ID), *message)
}

func (g *Gossiper) getMessage(origin string, id uint32) (*common.RumorMessage, bool) {
	val, ok := g.messages.Load(generateRumorUniqueString(nil, origin, id))
	if ok {
		msg := val.(common.RumorMessage)
		return &msg, ok
	}

	return nil, ok
}

// returns if the packet is valid (message ID is expected ID for the origin or 1 if origin
// is not known
func (g *Gossiper) isNewValidMessage(message *common.RumorMessage) bool {
	val, _ := g.clocks.LoadOrStore(message.Origin, uint32(1))

	return message.ID == val.(uint32)
}

func (g *Gossiper) peersString() string {
	peers := g.peers.Elements()
	return "PEERS " + strings.Join(peers, ",")
}

// generate a unique string of the host@id@origin or id@origin
// so host is optionnal (can be nil)
func generateRumorUniqueString(host *string, origin string, id uint32) string {
	if host != nil {
		return *host + "@" + fmt.Sprint(id) + "@" + origin
	} else {
		return fmt.Sprint(id) + "@" + origin
	}
}
