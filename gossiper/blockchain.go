package gossiper

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"math/rand"
	"sync"
	"time"
)

var GenesisBlockHash = [32]byte{}

type Blockchain struct {
	sync.RWMutex
	pendingTransactions         []common.TxPublish
	blocks                      sync.Map
	longestChainLastBlock       [32]byte
	currentHeight               uint64
	mappings                    sync.Map
	firstBroadcastBlockReceived bool
	changesNotifier             chan Notification
}

type Notification struct{}

func NewBlockchain() *Blockchain {
	return &Blockchain{
		pendingTransactions:   []common.TxPublish{},
		longestChainLastBlock: GenesisBlockHash,
		changesNotifier:       make(chan Notification, 1),
	}
}

func (bc *Blockchain) storeNewBlock(block *common.Block) {
	bc.blocks.Store(hex.EncodeToString(block.Hash()[:]), block)
}

func (bc *Blockchain) getBlock(hash [32]byte) (*common.Block, bool) {
	hashStr := hex.EncodeToString(hash[:])
	block, present := bc.blocks.Load(hashStr)
	if present {
		return block.(*common.Block), present
	}

	return nil, present
}

func (g *Gossiper) PublishTransaction(name string, size int64, metafileHash []byte) error {
	file := common.File{
		Name:         name,
		Size:         size,
		MetafileHash: metafileHash,
	}

	tx := common.TxPublish{
		File:     file,
		HopLimit: common.TxBroadcastHopLimit,
	}

	errs := common.BroadcastMessage(g.peers.Elements(), &tx, nil, g.gossipConn)
	if len(errs) > 0 {
		str := ""
		for counter, err := range errs {
			str += "(" + string(counter) + ") " + err.Error()
		}

		return errors.New("tx could not be delivered to some peers: " + str)
	}

	return nil
}

func (bc *Blockchain) HandleTx(tx common.TxPublish) {
	bc.Lock()
	defer bc.Unlock()
	for _, pendingTx := range bc.pendingTransactions {
		if tx.File.Name == pendingTx.File.Name {
			return
		}
	}

	alreadyClaimed := false

	bc.mappings.Range(func(nameRaw, _ interface{}) bool {
		name := nameRaw.(string)
		alreadyClaimed = name == tx.File.Name

		return !alreadyClaimed
	})

	if alreadyClaimed {
		return
	}

	bc.pendingTransactions = append(bc.pendingTransactions, tx)
}

func (bc *Blockchain) AddBlock(block *common.Block, minedLocally bool) bool {
	bc.Lock()
	defer bc.Unlock()

	height := uint64(1)
	_, prevBlockExists := bc.getBlock(block.PrevHash)

	forEachBlockInFork := func(block *common.Block, f func(*common.Block)) {
		prev, prevExists := bc.getBlock(block.PrevHash)
		for node, present := prev, prevExists; present; node, present = bc.getBlock(node.PrevHash) {
			f(node)
		}
	}

	// Check block validity
	if !powIsCorrect(block) {
		return false
	} else if !prevBlockExists && bc.firstBroadcastBlockReceived {
		return false
	} else {
		forEachBlockInFork(block, func(node *common.Block) {
			height++
			for _, tx := range node.Transactions {
				for _, newTx := range block.Transactions {
					if tx.File.Name == newTx.File.Name {
						return
					}
				}
			}
		})
	}
	if !minedLocally {
		bc.firstBroadcastBlockReceived = true
	}

	removeInvalidTransaction := func() {
		currentTxs := make([]common.TxPublish, len(bc.pendingTransactions))
		copy(currentTxs, bc.pendingTransactions)
		bc.pendingTransactions = make([]common.TxPublish, 0)
		for _, tx := range currentTxs {
			bc.HandleTx(tx)
		}
	}

	printChain := func(lastBlock *common.Block) {
		out := "CHAIN"
		forEachBlockInFork(lastBlock, func(node *common.Block) {
			out += " "
			out += fmt.Sprintf("%s:%s", hex.EncodeToString(node.Hash()[:]), hex.EncodeToString(node.PrevHash[:]))
			if len(node.Transactions) > 0 {
				txs := ""
				for _, tx := range node.Transactions {
					txs += fmt.Sprintf("%s:%s", tx.File.Name, hex.EncodeToString(tx.File.MetafileHash))
				}
				out += txs
			}
		})
		fmt.Println(out)
	}

	// Check if it creates a longer chain
	if block.PrevHash == bc.longestChainLastBlock {
		bc.addNewMappings(block.Transactions)
		bc.storeNewBlock(block)
		bc.currentHeight++
		bc.longestChainLastBlock = block.Hash()

		// Remove tx that have been added with this block
		removeInvalidTransaction()
		printChain(block)
	} else if height > bc.currentHeight {
		//
		// swap to longest fork
		//
		bc.mappings = sync.Map{}
		bc.storeNewBlock(block)
		forEachBlockInFork(block, func(node *common.Block) {
			bc.addNewMappings(block.Transactions)
		})
		bc.currentHeight = height

		//
		// Compute number of rewinded block
		//
		currentLastBlock, _ := bc.getBlock(bc.longestChainLastBlock)
		rewindedBlock := 0
		blockHeight := make(map[[32]byte]int)
		blockHeight[bc.longestChainLastBlock] = rewindedBlock
		// save height each block in current fork
		forEachBlockInFork(currentLastBlock, func(node *common.Block) {
			rewindedBlock++
			blockHeight[node.PrevHash] = rewindedBlock
		})
		// find the node where the fork happened
		forEachBlockInFork(block, func(node *common.Block) {
			exists := false
			rewindedBlock, exists = blockHeight[node.PrevHash]
			if exists {
				return
			}
		})

		// Switch chain
		bc.longestChainLastBlock = block.Hash()

		// Remove invalid transactions
		removeInvalidTransaction()
		fmt.Printf("FORK-LONGER rewind %d blocks\n", rewindedBlock)
		printChain(block)
	} else {
		bc.storeNewBlock(block)
		fmt.Printf("FORK-SHORTER %s\n", hex.EncodeToString(block.Hash()[:]))
	}

	return true
}

// Add new mappings. Should be called when a write lock on the Blockchain is taken !
func (bc *Blockchain) addNewMappings(txs []common.TxPublish) {
	bc.Lock()
	defer bc.Unlock()
	for _, tx := range txs {
		bc.mappings.Store(tx.File.Name, hex.EncodeToString(tx.File.MetafileHash))
	}
}

func (bc *Blockchain) getMetafileHashFromName(name string) (string, bool) {
	bc.RLock()
	defer bc.RUnlock()
	nameRaw, present := bc.mappings.Load(name)
	if !present {
		return "", present
	}

	return nameRaw.(string), present
}

func (bc *Blockchain) notifyMinerForChanges() {
	select {
	case bc.changesNotifier <- Notification{}:
	default:

	}
}

func (bc *Blockchain) getTransactions() []common.TxPublish {
	bc.RLock()
	defer bc.RUnlock()
	copied := make([]common.TxPublish, len(bc.pendingTransactions))
	copy(copied, bc.pendingTransactions)

	return copied
}

func (bc *Blockchain) startMining(minedBlocks chan<- *common.Block) {
	go func() {
		computingFirstBlock := true
		lastFoundBlockTime := time.Now()
		block := common.Block{
			Transactions: bc.getTransactions(),
			PrevHash:     bc.longestChainLastBlock,
		}
		for {
			select {
			case _ = <-bc.changesNotifier:
				block.Transactions = bc.getTransactions()
				block.PrevHash = bc.longestChainLastBlock
			default:
				rand.Read(block.Nonce[:])
				if powIsCorrect(&block) && bc.AddBlock(&block, true) {
					if computingFirstBlock {
						computingFirstBlock = false
						timer := time.NewTimer(common.FirstBlockPublicationDelay)
						<-timer.C
					} else {
						elapsedTime := time.Now().Sub(lastFoundBlockTime)
						timer := time.NewTimer(elapsedTime * 2)
						<-timer.C
						lastFoundBlockTime = time.Now()
					}
					minedBlocks <- &block
					fmt.Printf("FOUND-BLOCK %s\n", hex.EncodeToString(block.Hash()[:]))
				}
			}
		}
	}()
}

func powIsCorrect(block *common.Block) bool {
	hash := block.Hash()
	for i := 0; i < common.HashMinLeadingZeroBitsLength/8; i++ {
		if hash[i] != 0 {
			return false
		}
	}
	return true
}
