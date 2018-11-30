package gossiper

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"sort"
	"strings"
	"time"
)

func (g *Gossiper) propagateSearchRequest(packet *common.GossipPacket, from *string, decrementBudget bool) error {
	// Send to other nodes
	peers := g.peers.Elements()
	if from != nil {
		for counter, peer := range peers {
			if peer == *from {
				peers[counter] = peers[len(peers)-1]
				peers = peers[:len(peers)-1]
				break
			}
		}
	}
	peersCount := uint64(len(peers))
	srCopy := *packet.SearchRequest
	packet.SearchRequest = &srCopy
	if decrementBudget {
		packet.SearchRequest.Budget--
	}
	if packet.SearchRequest.Budget == 0 || peersCount == 0 {
		return nil
	} else if packet.SearchRequest.Budget < peersCount {
		var except []string
		if from == nil {
			except = []string{}
		} else {
			except = []string{*from}
		}
		selectedPeers, _ := g.peers.PickN(int(packet.SearchRequest.Budget), except)
		packet.SearchRequest.Budget = 1
		errs := common.BroadcastMessage(selectedPeers, packet, nil, g.gossipConn)
		for _, err := range errs {
			if err != nil {
				return errors.New("the search request cannot be sent to some hosts: " + err.Error())
			}
		}

		return nil
	} else {
		perHostBudget := packet.SearchRequest.Budget / peersCount
		supplementBudget := packet.SearchRequest.Budget % peersCount
		var err error = nil
		for _, peer := range peers {
			packet.SearchRequest.Budget = perHostBudget
			if supplementBudget > 0 {
				supplementBudget--
				packet.SearchRequest.Budget++
			}
			err = common.SendMessage(peer, packet, g.gossipConn)
		}
		if err != nil {
			return errors.New("the search request cannot be sent to some hosts: " + err.Error())
		}
	}

	return nil
}

func (g *Gossiper) replyToSearchRequest(packet *common.GossipPacket) error {
	results := g.data.SearchLocalFile(packet.SearchRequest.Keywords)
	if len(results) > 0 {
		gossipPacket := common.GossipPacket{
			SearchReply: &common.SearchReply{
				Origin:      g.name,
				Destination: packet.SearchRequest.Origin,
				HopLimit:    DefaultHopLimit,
				Results:     results,
			},
		}

		err := g.forwardPacket(&gossipPacket)
		if err != nil {
			return errors.New("cannot send search reply:" + err.Error())
		}
	}

	return nil
}

// process a search request. from is optional
func (g *Gossiper) ReplyAndPropagateSearchRequest(packet *common.GossipPacket, from *string) error {
	if packet.SearchRequest == nil {
		return errors.New("SearchRequest should not be nil")
	} else if g.isSearchRequestDuplicate(packet.SearchRequest) {
		return nil
	}

	g.addSearchRequestDuplicateTimer(packet.SearchRequest)

	err1 := g.replyToSearchRequest(packet)
	err := g.propagateSearchRequest(packet, from, true)

	if err != nil || err1 != nil {
		errStr := ""
		if err != nil {
			errStr += err.Error()
		}
		if err1 != nil {
			errStr += ", " + err1.Error()
		}
		return errors.New(errStr)
	}

	return nil
}

func (g *Gossiper) addSearchRequestDuplicateTimer(sr *common.SearchRequest) {
	ID := computeSearchRequestID(sr)
	g.waitSearchRequest.Store(ID)

	// Remove ID after SearchRequestDuplicateTimer time
	go func() {
		timer := time.NewTimer(common.SearchRequestDuplicateTimer)
		<-timer.C
		g.waitSearchRequest.Delete(ID)
	}()
}

func (g *Gossiper) isSearchRequestDuplicate(sr *common.SearchRequest) bool {
	ID := computeSearchRequestID(sr)

	return g.waitSearchRequest.Exists(ID)
}

func (g *Gossiper) searchRemoteFile(sr *common.SearchRequest) error {
	channel := g.addSearchReplyChannel(sr)
	validResult := CreateFilterFromKeywords(sr.Keywords)
	gossipPacket := &common.GossipPacket{
		SearchRequest: sr,
	}
	matches := common.NewConcurrentSet()
	exponentialIncrease := sr.Budget == 0
	endSearch := func() {
		g.removeSearchReplyChannel(sr)
		close(channel)
		fmt.Println("SEARCH FINISHED")
	}
	if exponentialIncrease {
		sr.Budget = common.DefaultBudget
	}

	// Listen for incoming reply to populate match list
	go func() {
		for {
			searchReply, more := <-channel
			if !more {
				return
			}
			for _, result := range searchReply.Results {
				// check if this search result came for this search request
				if validResult(result.FileName) {
					metafileHash := hex.EncodeToString(result.MetafileHash)
					g.data.addChunkLocation(metafileHash, result.FileName, result.ChunkMap, result.ChunkCount, searchReply.Origin)
					if g.data.remoteFileIsMatch(metafileHash) && !matches.Exists(metafileHash) {
						fmt.Printf("FOUND match %s at %s metafile=%s chunks=%s\n", result.FileName, searchReply.Origin, metafileHash, join(result.ChunkMap, ","))
						matches.Store(metafileHash)
					}
				}
			}
		}
	}()

	if exponentialIncrease {
		for len(matches.Elements()) < common.MatchThreshold && sr.Budget <= common.MaxBudget {
			err := g.propagateSearchRequest(gossipPacket, nil, false)
			if err != nil {
				endSearch()
				return err
			}
			timer := time.NewTimer(common.SearchRequestBudgetIncreasePeriod)
			<-timer.C
			sr.Budget *= 2
		}
		endSearch()
	} else {
		err := g.propagateSearchRequest(gossipPacket, nil, false)
		if err != nil {
			endSearch()
			return err
		}
		go func() {
			timer := time.NewTimer(common.RemoteSearchTimeout)
			<-timer.C
			endSearch()
		}()
	}

	return nil
}

func (g *Gossiper) addSearchReplyChannel(sr *common.SearchRequest) chan *common.SearchReply {
	channel := make(chan *common.SearchReply, 10)
	g.waitSearchReply.Store(computeSearchRequestID(sr), channel)

	return channel
}

func (g *Gossiper) removeSearchReplyChannel(sr *common.SearchRequest) {
	g.waitSearchReply.Delete(computeSearchRequestID(sr))
}

func (g *Gossiper) distributeSearchReply(sr *common.SearchReply) {
	g.waitSearchReply.Range(func(_, channelRaw interface{}) bool {
		channel := channelRaw.(chan *common.SearchReply)
		channel <- sr

		return true
	})
}

func computeSearchRequestID(sr *common.SearchRequest) string {
	var uniqueKeywords sort.StringSlice = common.Unique(sr.Keywords)
	uniqueKeywords.Sort()
	hash := sha1.New()
	for _, keyword := range uniqueKeywords {
		hash.Write([]byte(keyword))
	}
	keywordsID := hex.EncodeToString(hash.Sum(nil))
	ID := keywordsID + "@" + sr.Origin
	return ID
}

func join(ints []uint64, separator string) string {
	result := ""
	for _, key := range ints {
		result += fmt.Sprintf("%d%s", key, separator)
	}

	return strings.TrimSuffix(result, separator)
}
