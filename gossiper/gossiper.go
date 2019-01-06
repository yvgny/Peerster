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
	clientAddress     *net.UDPAddr
	gossipAddress     *net.UDPAddr
	clientConn        *net.UDPConn
	gossipConn        *net.UDPConn
	name              string
	rtimer            int
	peers             *common.ConcurrentSet
	clocks            *sync.Map
	waitCloudStorage  *sync.Map
	waitCloudRequest  *sync.Map
	waitAck           *sync.Map
	waitData          *sync.Map
	waitSearchRequest *common.ConcurrentSet
	waitSearchReply   *sync.Map
	messages          *sync.Map
	privateMessages   *Mail
	data              *DataManager
	routingTable      *RoutingTable
	cloudStorage      *CloudStorage
	simple            bool
	blockchain        *Blockchain
	keychain          *common.KeyStorage
	mutex             sync.Mutex
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

	dataManager, err := NewDataManager()
	if err != nil {
		return nil, err
	}

	g := &Gossiper{
		gossipAddress:     gAddress,
		clientAddress:     cAddress,
		clientConn:        cConn,
		gossipConn:        gConn,
		name:              name,
		peers:             peersSet,
		rtimer:            rtimer,
		clocks:            &clocksMap,
		waitCloudStorage:  &sync.Map{},
		waitCloudRequest:  &sync.Map{},
		waitAck:           &syncMap,
		waitData:          &sync.Map{},
		waitSearchRequest: common.NewConcurrentSet(),
		waitSearchReply:   &sync.Map{},
		messages:          &messagesMap,
		privateMessages:   newMail(),
		data:              dataManager,
		routingTable:      NewRoutingTable(),
		simple:            simpleBroadcastMode,
		blockchain:        NewBlockchain(),
	}

	ks, err := common.LoadKeyStorageFromDisk()
	if err != nil {
		ks, err = common.GenerateNewKeyStorage()
		if err != nil {
			return nil, err
		}

		err = ks.SaveKeyStorageOnDisk()
		if err != nil {
			return nil, err
		}
	}

	var cs *CloudStorage
	cs, err = LoadCloudStorageFromDisk()
	if err != nil {
		cs = CreateNewCloudStorage()
	}
	g.cloudStorage = cs

	g.keychain = ks

	g.startAntiEntropy(time.Duration(AntiEntropyPeriod) * time.Second)

	g.startRouteRumoring(time.Duration(rtimer) * time.Second)

	// Listen to blocks that are mined and broadcast them
	newBlocks := make(chan common.Block, 10)
	go func() {
		for {
			select {
			case block := <-newBlocks:
				packet := &common.GossipPacket{
					BlockPublish: &common.BlockPublish{
						Block:    block,
						HopLimit: common.BlockBroadcastHopLimit,
					},
				}
				common.BroadcastMessage(g.peers.Elements(), packet, nil, g.gossipConn)
			}
		}
	}()

	//g.blockchain.startMining(newBlocks)

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
					file, err := g.data.addLocalFile(filepath.Join(common.SharedFilesFolder, clientPacket.FileIndex.Filename), nil)
					if err != nil {
						fmt.Println(err.Error())
					}
					hashSlice, err := hex.DecodeString(file.MetaHash)
					if err != nil {
						fmt.Println("Cannot publish transaction: " + err.Error())
						return
					}
					tx := common.TxPublish{
						File: &common.File{
							Size:         file.Size,
							Name:         file.Name,
							MetafileHash: hashSlice,
						},
					}
					if valid := g.blockchain.HandleTx(tx); valid {
						_ = g.PublishTransaction(file.Name, file.Size, hashSlice)
						fmt.Printf("Added new file from %s with hash %s\n", clientPacket.FileIndex.Filename, file.MetaHash)
					} else {
						fmt.Println("Cannot index file: name already exists in blockchain")
					}
				} else if clientPacket.FileDownload != nil {
					err = g.downloadFile(clientPacket.FileDownload.User, clientPacket.FileDownload.HashValue, clientPacket.FileDownload.Filename, nil)
					if err != nil {
						fmt.Println(err.Error())
					}
				} else if clientPacket.SearchRequest != nil {
					clientPacket.SearchRequest.Origin = g.name
					err = g.searchRemoteFile(clientPacket.SearchRequest)
					if err != nil {
						fmt.Println("Could not search file: " + err.Error())
					}
				} else if clientPacket.CloudPacket != nil {
					println("RECEIVED CLOUDPACKET")
					filename := clientPacket.CloudPacket.Filename
					err = g.HandleClientCloudRequest(filename)
					if err != nil {
						fmt.Println(err.Error())
						return
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
						data, err := g.data.getLocalData(gossipPacket.DataRequest.HashValue)
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
							print("SEARCHING FOR HASH : ")
							println(hex.EncodeToString(gossipPacket.DataRequest.HashValue))
							print("FROM USER ")
							println(gossipPacket.DataRequest.Origin)
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
				} else if gossipPacket.SearchRequest != nil {
					addrString := addr.String()
					err = g.ReplyAndPropagateSearchRequest(gossipPacket, &addrString)
					if err != nil {
						fmt.Println(errors.New("error while processing search request: " + err.Error()))
						return
					}
				} else if gossipPacket.SearchReply != nil {
					if gossipPacket.SearchReply.Destination == g.name {
						g.distributeSearchReply(gossipPacket.SearchReply)
					} else {
						err = g.forwardPacket(gossipPacket)
						if err != nil {
							fmt.Println("Could not forward search reply: " + err.Error())
							return
						}
					}
				} else if gossipPacket.TxPublish != nil {
					gossipPacket.TxPublish.File.MetafileHash = append([]byte(nil), gossipPacket.TxPublish.File.MetafileHash...)
					valid := g.blockchain.HandleTx(*gossipPacket.TxPublish)
					gossipPacket.TxPublish.HopLimit--
					if valid && gossipPacket.TxPublish.HopLimit > 0 {
						addrStr := addr.String()
						common.BroadcastMessage(g.peers.Elements(), gossipPacket, &addrStr, g.gossipConn)
					}
				} else if gossipPacket.BlockPublish != nil {
					valid := g.blockchain.AddBlock(gossipPacket.BlockPublish.Block, false)
					gossipPacket.BlockPublish.HopLimit--
					if valid && gossipPacket.BlockPublish.HopLimit > 0 {
						addrStr := addr.String()
						common.BroadcastMessage(g.peers.Elements(), gossipPacket, &addrStr, g.gossipConn)
					}
				} else if ack := gossipPacket.FileUploadAck; ack != nil {
					println("RECEIVED FILEUPLOADACK")
					if ack.Destination != g.name {
						err := g.forwardPacket(gossipPacket)
						if err != nil {
							fmt.Println("Could not forward packet for " + ack.Destination + " : " + err.Error())
						}
						return
					}
					metaHashStr := hex.EncodeToString(gossipPacket.FileUploadAck.MetaHash[:])
					channel, exist := g.waitCloudStorage.Load(metaHashStr)
					if exist {
						channel.(chan *common.FileUploadAck) <- ack
					}
				} else if message := gossipPacket.FileUploadMessage; message != nil {
					println("RECEIVED FILEUPLOADMESSAGE")
					metaHash, metaFile, dest := message.MetaHash, message.MetaFile, message.Origin
					err = g.data.addLocalData(metaFile, metaHash[:])
					if err != nil {
						fmt.Println("Could not add local data : " + err.Error())
						return
					}
					downloadedChunks := make([]uint64, 0)
					for i := 1; i < len(message.MetaFile)/sha256.Size ; i++ {
						for _, chunk := range message.UploadedChunks {
							if chunk == uint64(i) {
								continue
							}
						}
						// Without loading the metafile everytime, the metafile content is modified
						metaFile, err = g.data.getLocalData(metaHash[:])
						if err != nil {
							fmt.Println("unable to load metafile : " + err.Error())
						}
						err := g.downloadChunk(dest, metaFile[(i - 1)*sha256.Size:i*sha256.Size], i)
						if err != nil {
							fmt.Printf("Could not download chunk %d : %s\n", i, err.Error())
						} else {
							downloadedChunks = append(downloadedChunks, uint64(i))
						}
					}
					ack := common.FileUploadAck{
						Origin:         g.name,
						Destination:    dest,
						UploadedChunks: downloadedChunks,
						MetaHash:       metaHash,
					}
					chunksHash, err := g.data.HashChunksOfLocalFile(metaHash[:], downloadedChunks, sha256.New())
					if err != nil {
						fmt.Printf("Could not hash chunks: %s\n", err.Error())
						return
					}
					ack.Sign(g.keychain.AsymmetricPrivKey, message.Nonce, chunksHash)
					hop, exist := g.routingTable.getNextHop(dest)
					if exist {
						if err := common.SendMessage(hop, &common.GossipPacket{FileUploadAck: &ack}, g.gossipConn); err != nil {
							fmt.Println("Could not ack FileUploadMessage : " + err.Error())
							return
						}
					}
					if len(downloadedChunks)+len(message.UploadedChunks) < len(metaFile)/sha256.Size {
						message.HopLimit = message.HopLimit - 1
						if message.HopLimit < 1 {
							return
						}
						for _, chunk := range downloadedChunks {
							message.UploadedChunks = append(message.UploadedChunks, chunk)
						}
						peer, exist := g.peers.PickN(1, []string{addr.String()})
						if exist {
							if err := common.SendMessage(peer[0], &common.GossipPacket{FileUploadAck: &ack}, g.gossipConn); err != nil {
								fmt.Println("Could not forward FileUploadMessage : " + err.Error())
								return
							}
						}
					}
				} else if request := gossipPacket.UploadedFileRequest; request != nil {
					println("RECEIVED UPLOADEDFILEREQUEST")
					nonce, dest, metaHash := request.Nonce, request.Origin, request.MetaHash
					//TODO Forward messages
					metaFile, err := g.data.getLocalData(metaHash[:])
					if err != nil {
						fmt.Println("Could not get local record : " + err.Error())
						return //File does not exist locally
					}
					//TODO Add signature of the UploadedFileRequest if time allows
					reply := common.UploadedFileReply{
						Origin:      g.name,
						OwnedChunks: g.getOwnedChunks(metaFile),
						Destination: dest,
						HopLimit:    DefaultHopLimit,
						MetaHash:    metaHash,
					}
					reply.Sign(g.keychain.AsymmetricPrivKey, nonce)
					hop, exist := g.routingTable.getNextHop(dest)
					if exist {
						if err := common.SendMessage(hop, &common.GossipPacket{UploadedFileReply: &reply}, g.gossipConn); err != nil {
							fmt.Println("Could not reply to UploadedFileRequest : " + err.Error())
						}
					}
				} else if reply := gossipPacket.UploadedFileReply; reply != nil {
					println("RECEIVED UPLOADEDFILEREPLY")
					//Check when we can reconstruct the file and trigger download when we all chunks somewhere
					dest := reply.Destination
					if dest != g.name {
						println("Forwarding packet to " + dest)
						if err := g.forwardPacket(gossipPacket); err != nil {
							fmt.Println("Could not forward UploadedFileReply : " + err.Error())
						}
						return
					}
					channel, exist := g.waitCloudRequest.Load(hex.EncodeToString(reply.MetaHash[:]))
					if !exist {
						fmt.Println("Cannot find channel for the cloud request.")
						return
					}
					channel.(chan *common.UploadedFileReply) <- reply
				}
			}()
		}
	}()
}

func (g *Gossiper) getOwnedChunks(metaFile []byte) []uint64 {
	chunkMap := make([]uint64, 0)
	for i := 1 ; i < len(metaFile) / sha256.Size ; i++ {
		_, err := g.data.getLocalData(metaFile[(i - 1) * sha256.Size:i * sha256.Size])
		if err == nil {
			chunkMap = append(chunkMap, uint64(i))
		}
	}
	return chunkMap
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
	} else if packet.SearchReply != nil {
		dest = packet.SearchReply.Destination
		hopCount = packet.SearchReply.HopLimit - 1
	} else if packet.UploadedFileReply != nil {
		dest = packet.UploadedFileReply.Destination
		hopCount = packet.UploadedFileReply.HopLimit - 1
		packet.UploadedFileReply.HopLimit = packet.UploadedFileReply.HopLimit - 1
	} else if packet.FileUploadAck != nil {
		dest = packet.FileUploadAck.Destination
		hopCount = packet.FileUploadAck.HopLimit - 1
		packet.FileUploadAck.HopLimit = packet.FileUploadAck.HopLimit - 1
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

func (g *Gossiper) downloadFile(user string, hash []byte, filename string, key *[32]byte) error {
	metafileHash := hex.EncodeToString(hash)

	// returns the the tuple (peer_name, next_hop, valid)
	getNextHop := func(chunkNbc uint64) (string, string, bool) {
		if user != "" {
			address, valid := g.routingTable.getNextHop(user)
			return user, address, valid
		}
		host, ok := g.data.getRandomChunkLocation(metafileHash, chunkNbc)
		if !ok {
			return "", "", false
		}
		nextHop, valid := g.routingTable.getNextHop(host)
		return host, nextHop, valid
	}

	go func() {
		packet := common.GossipPacket{
			DataRequest: &common.DataRequest{
				Origin:    g.name,
				HopLimit:  DefaultHopLimit,
				HashValue: hash,
			},
		}

		replyChan := make(chan *common.DataReply)
		g.waitData.Store(metafileHash, &replyChan)
		fmt.Printf("DOWNLOADING metafile of %s from %s\n", filename, user)
		// retry every period to get the meta file
		for try := 0; try < MaxChunkDownloadRetryLimit; try++ {
			metafilePeer, metafileNextHop, ok := getNextHop(1)
			if !ok {
				fmt.Println("cannot find next hop for metafile (using hop of chunk 0)")
				return
			}
			packet.DataRequest.Destination = metafilePeer
			err := common.SendMessage(metafileNextHop, &packet, g.gossipConn)
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
				err = g.data.addLocalData(metafile, reply.HashValue)
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
				chunkList := make([]uint64, 0)
				// get every chunk
				for i := 0; i < len(metafile); i += sha256.Size {
					chunkNr := uint64(i/sha256.Size + 1)
					packet.DataRequest.HashValue = metafile[i : i+sha256.Size]
					chunckHex := hex.EncodeToString(packet.DataRequest.HashValue)
					g.waitData.Store(chunckHex, &replyChan)
					peer, nextHop, ok := getNextHop(chunkNr)
					fmt.Printf("DOWNLOADING %s chunk %d from %s\n", filename, (i/sha256.Size)+1, peer)
					if !ok {
						fmt.Printf("cannot find next hop for chunk %d\n", i)
						continue
					}
					packet.DataRequest.Destination = peer
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
							err = g.data.addLocalData(chunck.Data, chunck.HashValue)
							if err != nil {
								fmt.Println(errors.New("skipping chunk: cannot download chunk: " + err.Error()))
								break loop
							}
							toFile := chunck.Data
							if key != nil {
								toFile, err = common.DecryptChunk(chunck.Data, *key)
								//TODO : Verify fix is correct
								if err != nil {
									fmt.Println(errors.New("skipping chunk: cannot download chunk: " + err.Error()))
									break loop
								}
							}
							_, err = f.Write(toFile)
							if err != nil {
								fmt.Println(errors.New("Unable to write in file: " + err.Error()))
							}
							chunkList = append(chunkList, chunkNr)
							break loop
						case <-timer.C:
						}

					}
				}

				_ = f.Sync()
				_ = f.Close()

				// TODO: change the size to the correct one
				g.data.addLocalRecord(metafileHash, filename, chunkList, uint64(len(metafile)/sha256.Size), 0)
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

func (g *Gossiper) downloadChunk(dest string, hash []byte, i int) error {
	hashStr := hex.EncodeToString(hash)

	packet := common.GossipPacket{
		DataRequest: &common.DataRequest{
			Origin:    g.name,
			HopLimit:  DefaultHopLimit,
			HashValue: hash,
		},
	}

	replyChan := make(chan *common.DataReply)
	g.waitData.Store(hashStr, &replyChan)
	fmt.Printf("DOWNLOADING chunk %d from %s\n", i, dest)
	// retry every period to get the meta file
	for try := 0; try < MaxChunkDownloadRetryLimit; try++ {
		packet.DataRequest.Destination = dest
		nextHop, exist := g.routingTable.getNextHop(dest)
		if !exist {
			fmt.Println("Cannot send dataRequest to ", dest)
		}
		err := common.SendMessage(nextHop, &packet, g.gossipConn)
		if err != nil {
			return err
		}

		timer := time.NewTimer(DataReplyTimeOut)
		select {
		case reply := <-replyChan:
			data := make([]byte, len(reply.Data))
			copy(data, reply.Data)
			g.waitData.Delete(hashStr)
			if hex.EncodeToString(reply.HashValue) != hashStr {
				return errors.New("cannot download chunk : hash doesn't match with reply")
			}
			err = g.data.addLocalData(data, reply.HashValue)
			if err != nil {
				fmt.Println(errors.New("cannot download chunk: " + err.Error()))
			}

			fmt.Printf("DOWNLOADED chunk %d from %s\n", i, dest)
			return nil
		case <-timer.C:
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
