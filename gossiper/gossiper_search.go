package gossiper

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"github.com/yvgny/Peerster/common"
	"sort"
	"time"
)

func (g *Gossiper) processSearchRequest(packet *common.GossipPacket, from string) error {
	if packet.SearchRequest == nil {
		return errors.New("SearchRequest should not be nil")
	} else if g.isSearchRequestDuplicate(packet.SearchRequest) {
		return nil
	}

	results := g.data.SearchFile(packet.SearchRequest.Keywords)
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

	// Send to other nodes
	peers := g.peers.Elements()
	peersCount := uint64(len(peers))
	packet.SearchRequest.Budget--
	if packet.SearchRequest.Budget == 0 {
		return nil
	} else if packet.SearchRequest.Budget < peersCount {
		selectedPeers, _ := g.peers.PickN(int(packet.SearchRequest.Budget), []string{from})
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

func (g *Gossiper) addSearchResultTimer(sr *common.SearchRequest) {
	ID := computeSearchRequestID(sr)
	g.waitSearchRequest.Store(ID)

	// Remove ID after SearchRequestTimer time
	go func() {
		timer := time.NewTimer(common.SearchRequestTimer)
		<-timer.C
		g.waitSearchRequest.Delete(ID)
	}()
}

func (g *Gossiper) isSearchRequestDuplicate(sr *common.SearchRequest) bool {
	ID := computeSearchRequestID(sr)

	return g.waitSearchRequest.Exists(ID)
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
