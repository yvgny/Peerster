package gossiper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/yvgny/Peerster/common"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const DefaultHopLimit uint32 = 10
const DefaultUdpBufferSize int = 12228
const DefaultChunkSize int = 8192
const DataReplyTimeOut = 5 * time.Second
const AntiEntropyPeriod int = 1
const DownloadFolder = "_Downloads"
const MaxChunkDownloadRetryLimit = 10

type Gossiper struct {
	clientAddress   *net.UDPAddr
	gossipAddress   *net.UDPAddr
	clientConn      *net.UDPConn
	gossipConn      *net.UDPConn
	name            string
	rtimer          int
	peers           *common.ConcurrentSet
	clocks          *sync.Map
	waitAck         *sync.Map
	waitData        *sync.Map
	messages        *sync.Map
	privateMessages *Mail
	data            *DataManager
	routingTable    *RoutingTable
	simple          bool
	mutex           sync.Mutex
}

func NewGossiper(clientAddress, gossipAddress, name, peers string, simpleBroadcastMode bool, rtimer int) (*Gossiper, error) {
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
		gossipAddress:   gAddress,
		clientAddress:   cAddress,
		clientConn:      cConn,
		gossipConn:      gConn,
		name:            name,
		peers:           peersSet,
		rtimer:          rtimer,
		clocks:          &clocksMap,
		waitAck:         &syncMap,
		waitData:        &sync.Map{},
		messages:        &messagesMap,
		privateMessages: newMail(),
		data:            NewDataManager(),
		routingTable:    NewRoutingTable(),
		simple:          simpleBroadcastMode,
	}

	g.startAntiEntropy(time.Duration(AntiEntropyPeriod) * time.Second)

	g.startRouteRumoring(time.Duration(rtimer) * time.Second)

	return g, nil
}

// Start two listeners : one for the client side (listening on UIPort) and one
// for the peers (listening on gossipAddr)
func (g *Gossiper) StartGossiper() {
	// Handle clients messages
	go func() {
		buffer := make([]byte, DefaultUdpBufferSize)
		for {
			n, _, err := g.clientConn.ReadFromUDP(buffer)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			clientPacket := &common.ClientPacket{}
			err = protobuf.Decode(buffer[:n], clientPacket)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}

			// Handle the packet in a new thread to be able to listen on new messages again
			go func() {
				if clientPacket.Rumor != nil {
					if err = g.HandleClientRumorMessage(clientPacket); err != nil {
						fmt.Println(err.Error())
					}
				} else if clientPacket.Simple != nil && g.simple {
					output := fmt.Sprintf("CLIENT MESSAGE %s\n", clientPacket.Simple.Contents)
					fmt.Println(output + g.peersString())
					gossipPacket := &common.GossipPacket{}
					gossipPacket.Simple = clientPacket.Simple
					clientPacket.Simple.OriginalName = g.name
					errorList := common.BroadcastMessage(g.peers.Elements(), gossipPacket, nil, g.gossipConn)
					if errorList != nil {
						for _, err = range errorList {
							fmt.Println(err.Error())
						}
					}
				} else if packet := clientPacket.Private; packet != nil {
					err = g.sendPrivateMessage(packet.Destination, packet.Text)
					if err != nil {
						fmt.Println(err.Error())
					}
				} else if clientPacket.FileIndex != nil {
					hash, err := g.data.addFile(filepath.Join(common.SharedFilesFolder, clientPacket.FileIndex.Filename))
					if err != nil {
						fmt.Println(err.Error())
					}
					fmt.Printf("Added new file from %s with hash %s\n", clientPacket.FileIndex.Filename, hex.EncodeToString(hash))
				} else if clientPacket.FileDownload != nil {
					err = g.downloadFile(clientPacket.FileDownload.User, clientPacket.FileDownload.HashValue, clientPacket.FileDownload.Filename)
					if err != nil {
						fmt.Println(err.Error())
					}
				}
			}()
		}
	}()

	// Handle peers messages
	go func() {
		buffer := make([]byte, DefaultUdpBufferSize)
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
					// Remove route rumor messages
					if gossipPacket.Rumor.Text != "" {
						output := fmt.Sprintf("RUMOR origin %s from %s ID %d contents %s\n", gossipPacket.Rumor.Origin, addr, gossipPacket.Rumor.ID, gossipPacket.Rumor.Text)
						fmt.Println(output + g.peersString())
					}

					if g.isNewValidMessage(gossipPacket.Rumor) {
						//g.incrementClock(gossipPacket.Rumor.Origin)
						g.clocks.Store(gossipPacket.Rumor.Origin, gossipPacket.Rumor.ID+1)
						g.storeMessage(gossipPacket.Rumor)

						// Update DSDV table
						if gossipPacket.Rumor.Origin != g.name {
							g.routingTable.updateRoute(gossipPacket.Rumor.Origin, addr.String())
							fmt.Printf("DSDV %s %s\n", gossipPacket.Rumor.Origin, addr.String())
						}

						// Send ack
						if err = g.sendStatusPacket(addr.String()); err != nil {
							fmt.Println(err.Error())
							return
						}

						if err = g.startMongering(gossipPacket, nil, nil); err != nil {
							fmt.Println(err.Error())
							return
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
							return
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
					if len(errList) > 0 {
						return
					}
				} else if gossipPacket.Private != nil {
					packet := gossipPacket.Private
					if packet.Destination == g.name {
						fmt.Printf("PRIVATE origin %s hop-limit %d contents %s\n", packet.Origin, packet.HopLimit, packet.Text)
						g.privateMessages.addMessage(packet.Origin, packet.Destination, packet.Text)
					} else {
						err = g.forwardPacket(gossipPacket)
						if err != nil {
							fmt.Println(err.Error())
							return
						}
					}
				} else if gossipPacket.DataRequest != nil {
					if gossipPacket.DataRequest.Destination == g.name {
						data, err := g.data.getData(gossipPacket.DataRequest.HashValue)
						if err == nil {
							reply := common.GossipPacket{
								DataReply: &common.DataReply{
									Origin:      g.name,
									Destination: gossipPacket.DataRequest.Origin,
									HopLimit:    DefaultHopLimit,
									HashValue:   gossipPacket.DataRequest.HashValue,
									Data:        data,
								},
							}

							nexthop, ok := g.routingTable.getNextHop(gossipPacket.DataRequest.Origin)
							if !ok {
								fmt.Println(errors.New("next hop for destination " + gossipPacket.DataRequest.Origin + " not known"))
								return
							}
							err = common.SendMessage(nexthop, &reply, g.gossipConn)
							if err != nil {
								fmt.Println(err.Error())
								return
							}

						} else {
							fmt.Println(errors.New("unable to load requested chunk:" + err.Error()))
							return
						}
					} else {
						err = g.forwardPacket(gossipPacket)
						if err != nil {
							fmt.Println(err.Error())
							return
						}
					}
				} else if gossipPacket.DataReply != nil {
					if gossipPacket.DataReply.Destination == g.name {
						hexHash := hex.EncodeToString(gossipPacket.DataReply.HashValue)
						if chanRaw, ok := g.waitData.Load(hexHash); ok {
							channel := chanRaw.(*chan *common.DataReply)
							*channel <- gossipPacket.DataReply
						}
					} else {
						err = g.forwardPacket(gossipPacket)
						if err != nil {
							fmt.Println(err.Error())
							return
						}
					}
				}
			}()
		}
	}()
}

func (g *Gossiper) forwardPacket(packet *common.GossipPacket) error {
	var dest string
	var hopCount uint32
	if packet.Private != nil {
		dest = packet.Private.Destination
		hopCount = packet.Private.HopLimit - 1
	} else if packet.DataRequest != nil {
		dest = packet.DataRequest.Destination
		hopCount = packet.DataRequest.HopLimit - 1
	} else if packet.DataReply != nil {
		dest = packet.DataReply.Destination
		hopCount = packet.DataReply.HopLimit - 1
	} else {
		return errors.New("cannot forward packet of this type")
	}

	if hopCount <= 0 {
		return nil
	}

	nexthop, ok := g.routingTable.getNextHop(dest)
	if !ok {
		return errors.New(fmt.Sprintf("cannot find next hop for destination %s when forwarding the packet", dest))
	}

	err := common.SendMessage(nexthop, packet, g.gossipConn)
	if err != nil {
		return err
	}

	return nil
}

func (g *Gossiper) downloadFile(user string, hash []byte, filename string) error {
	nextHop, ok := g.routingTable.getNextHop(user)
	if !ok {
		return errors.New("no route to host " + nextHop)
	}

	go func() {
		packet := common.GossipPacket{
			DataRequest: &common.DataRequest{
				Origin:      g.name,
				Destination: user,
				HopLimit:    DefaultHopLimit,
				HashValue:   hash,
			},
		}

		replyChan := make(chan *common.DataReply)
		metafileHash := hex.EncodeToString(hash)
		g.waitData.Store(metafileHash, &replyChan)
		fmt.Printf("DOWNLOADING metafile of %s from %s\n", filename, user)
		// retry every period to get the meta file
		for try := 0; try < MaxChunkDownloadRetryLimit; try++ {
			err := common.SendMessage(nextHop, &packet, g.gossipConn)
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			timer := time.NewTimer(DataReplyTimeOut)
			select {
			case reply := <-replyChan:
				metafile := make([]byte, len(reply.Data))
				copy(metafile, reply.Data)
				g.waitData.Delete(metafileHash)
				if hex.EncodeToString(reply.HashValue) != metafileHash {
					fmt.Println(errors.New("cannot download metafile : hash doesn't match with reply"))
					return
				}
				err = g.data.addData(metafile, reply.HashValue)
				if err != nil {
					fmt.Println(errors.New("cannot download metafile: " + err.Error()))
				}
				// create file
				pathStr := filepath.Join(DownloadFolder, filename)
				f, err := os.Create(pathStr)
				if err != nil {
					fmt.Println(err)
					return
				}
				// get every chunk
				for i := 0; i < len(metafile); i += sha256.Size {
					packet.DataRequest.HashValue = metafile[i : i+sha256.Size]
					chunckHex := hex.EncodeToString(packet.DataRequest.HashValue)
					g.waitData.Store(chunckHex, &replyChan)
					fmt.Printf("DOWNLOADING %s chunk %d from %s\n", filename, (i/sha256.Size)+1, user)

					// retry every period until chunk is downloaded
				loop:
					for try1 := 0; try1 < MaxChunkDownloadRetryLimit; try1++ {
						err = common.SendMessage(nextHop, &packet, g.gossipConn)
						if err != nil {
							fmt.Println(err.Error())
							return
						}
						timer = time.NewTimer(DataReplyTimeOut)
						select {
						case chunck := <-replyChan:
							g.waitData.Delete(chunckHex)
							if hex.EncodeToString(chunck.HashValue) != chunckHex {
								fmt.Println(errors.New("skipping chunk: cannot download chunk: hash mismatch"))
								break loop
							}
							err = g.data.addData(chunck.Data, chunck.HashValue)
							if err != nil {
								fmt.Println(errors.New("skipping chunk: cannot download chunk: " + err.Error()))
								break loop
							}
							_, err = f.Write(chunck.Data)
							if err != nil {
								fmt.Println(errors.New("Unable to write in file: " + err.Error()))
							}
							break loop
						case <-timer.C:
						}

					}
				}

				f.Sync()
				f.Close()

				fmt.Printf("RECONSTRUCTED file %s\n", filename)

				return
			case <-timer.C:
			}
		}

	}()

	return nil
}

func (g *Gossiper) sendPrivateMessage(destination, text string) error {
	packet := &common.PrivateMessage{
		Destination: destination,
		Text:        text,
		Origin:      g.name,
		ID:          0,
		HopLimit:    DefaultHopLimit,
	}
	gossipPacket := &common.GossipPacket{
		Private: packet,
	}

	g.privateMessages.addMessage(g.name, destination, text)

	if hop, ok := g.routingTable.getNextHop(packet.Destination); ok {
		if err := common.SendMessage(hop, gossipPacket, g.gossipConn); err != nil {
			return err
		}
	}

	return nil
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

func (g *Gossiper) decrementClock(peer string) uint32 {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	currentClockInter, loaded := g.clocks.LoadOrStore(g.name, uint32(1))
	currentClock := currentClockInter.(uint32)

	if loaded {
		return currentClock
	}
	g.clocks.Store(g.name, currentClock-1)

	return currentClock
}

func (g *Gossiper) HandleClientRumorMessage(packet *common.ClientPacket) error {
	packet.Rumor.Origin = g.name
	currentClock := g.incrementClock(g.name)
	output := fmt.Sprintf("CLIENT MESSAGE %s\n", packet.Rumor.Text)
	fmt.Println(output + g.peersString())
	packet.Rumor.ID = currentClock
	g.storeMessage(packet.Rumor)
	gossipPacket := &common.GossipPacket{}
	gossipPacket.Rumor = packet.Rumor

	if err := g.startMongering(gossipPacket, nil, nil); err != nil {
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
	if period > 0 {
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
}

func (g *Gossiper) startRouteRumoring(period time.Duration) {
	if period > 0 {
		routeMsg := common.GossipPacket{
			Rumor: &common.RumorMessage{
				Origin: g.name,
			},
		}
		sendMsg := func() {
			routeMsg.Rumor.ID = g.incrementClock(g.name)
			for _, host := range g.peers.Elements() {
				if err := common.SendMessage(host, &routeMsg, g.gossipConn); err != nil {
					fmt.Println(err.Error())
				}
			}
			g.storeMessage(routeMsg.Rumor)
		}
		// Send a first announcement
		sendMsg()

		// Then at each period
		go func() {
			ticker := time.NewTicker(period)
			for range ticker.C {
				sendMsg()
			}
		}()
	}

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
			// unable to find a new peer to monger with
			return nil
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
			msg, loaded := g.getMessage(peerStatus.Identifier, peerStatus.NextID)
			if !loaded {
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

	neededUpdate := false
	g.clocks.Range(func(peerVal, _ interface{}) bool {
		peerID := peerVal.(string)
		for _, peerStatus := range status.Want {
			if peerStatus.Identifier == peerID {
				return true
			}
		}
		msg, loaded := g.getMessage(peerID, uint32(1))
		if !loaded {
			return true
		}
		gossipPack := common.GossipPacket{
			Rumor: msg,
		}
		if err := g.startMongering(&gossipPack, &peer, nil); err != nil {
			neededUpdate = true
			return false
		}
		return true
	})

	if neededUpdate {
		return true, nil
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