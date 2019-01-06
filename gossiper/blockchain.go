package gossiper

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var GenesisBlockHash = [32]byte{}

type Blockchain struct {
	sync.RWMutex
	pendingTransactions   []common.TxPublish
	blocks                sync.Map
	longestChainLastBlock [32]byte
	currentHeight         uint64
	mappings              sync.Map
	changesNotifier       chan Notification
	pubKeyMapping         sync.Map
	claimedPubkey		  common.ConcurrentSet
}

type Notification struct{}

func NewBlockchain() *Blockchain {
	return &Blockchain{
		pendingTransactions:   []common.TxPublish{},
		longestChainLastBlock: GenesisBlockHash,
		changesNotifier:       make(chan Notification, 1),
		claimedPubkey:         *common.NewConcurrentSet(),
	}
}

func (bc *Blockchain) storeNewBlock(block common.Block) {
	hash := block.Hash()
	block.Transactions = append([]common.TxPublish(nil), block.Transactions...)
	bc.blocks.Store(hex.EncodeToString(hash[:]), block)
}

func (bc *Blockchain) getBlock(hash [32]byte) (*common.Block, bool) {
	hashStr := hex.EncodeToString(hash[:])
	blockRaw, present := bc.blocks.Load(hashStr)
	if present {
		block := blockRaw.(common.Block)
		block.Transactions = append([]common.TxPublish(nil), block.Transactions...)
		return &block, present
	}

	return nil, present
}

// Make sure tx is a deep copy of the origin TxPublish when calling PublishTransaction
func (g *Gossiper) PublishTransaction(tx common.TxPublish) error {
	tx.HopLimit = common.TxBroadcastHopLimit

	packet := common.GossipPacket{
		TxPublish: &tx,
	}

	errs := common.BroadcastMessage(g.peers.Elements(), &packet, nil, g.gossipConn)
	if len(errs) > 0 {
		str := ""
		for counter, err := range errs {
			str += "(" + string(counter) + ") " + err.Error() + " "
		}

		return errors.New("tx could not be delivered to some peers: " + str)
	}

	return nil
}

// return true if transaction has been added (= valid + not seen for the moment)
func (bc *Blockchain) HandleTx(tx common.TxPublish) bool {
	if tx.Mapping != nil && !tx.Mapping.VerifySignature() {
		fmt.Println("tx signature not verified")
		return false
	}
	bc.Lock()
	defer bc.Unlock()
	return bc.handleTxWithoutLock(tx)
}

func txClaimTheSame(txA common.TxPublish, txB common.TxPublish) bool {
	return (txA.File != nil && txB.File != nil && txA.File.Name == txB.File.Name) ||
		   (txA.Mapping != nil && txB.Mapping != nil && bytes.Equal(txA.Mapping.PublicKey, txB.Mapping.PublicKey))
}

// return true if transaction has been added (= valid + not seen for the moment)
func (bc *Blockchain) handleTxWithoutLock(tx common.TxPublish) bool {
	for _, pendingTx := range bc.pendingTransactions {
		if txClaimTheSame(tx, pendingTx) {
			return false
		}
	}

	alreadyClaimed := false
	if tx.File != nil {
		_, alreadyClaimed = bc.mappings.Load(tx.File.Name)
	}
	if tx.Mapping != nil {
		alreadyClaimed = alreadyClaimed || bc.claimedPubkey.Exists(hex.EncodeToString(tx.Mapping.PublicKey))
	}

	if alreadyClaimed {
		return false
	}

	bc.pendingTransactions = append(bc.pendingTransactions, tx)
	bc.notifyMinerForChanges()

	return true
}

// return true is block is valid and added to the chain, or one if its fork
func (bc *Blockchain) AddBlock(block common.Block, minedLocally bool) bool {
	bc.Lock()
	defer bc.Unlock()

	block = *block.Clone()

	height := uint64(1)
	prevBlock, prevBlockExists := bc.getBlock(block.PrevHash)
	_, blockExists := bc.getBlock(block.Hash())
	if blockExists {
		return false
	}

	// f should return true to continue the iteration. False will stop.
	forEachBlockInFork := func(lastBlock *common.Block, f func(*common.Block) bool) {
		for node, present := lastBlock, lastBlock != nil; present; node, present = bc.getBlock(node.PrevHash) {
			shouldContinue := f(node)
			if !shouldContinue {
				return
			}
		}
	}

	// Check block validity
	if !powIsCorrect(&block) {
		return false
	} else if prevBlockExists {
		alreadyClaimed := false
		forEachBlockInFork(prevBlock, func(node *common.Block) bool {
			height++
			for _, tx := range node.Transactions {
				for _, newTx := range block.Transactions {
					if txClaimTheSame(newTx, tx) {
						alreadyClaimed = true
						return false
					}
				}
			}
			return true
		})
		if alreadyClaimed {
			return false
		}
	}
	removeInvalidTransaction := func() {
		currentTxs := append([]common.TxPublish(nil), bc.pendingTransactions...)
		bc.pendingTransactions = make([]common.TxPublish, 0)
		for _, tx := range currentTxs {
			bc.handleTxWithoutLock(tx)
		}
	}

	printChain := func(lastBlock *common.Block) {
		out := "CHAIN"
		forEachBlockInFork(lastBlock, func(node *common.Block) bool {
			out += " "
			hash := node.Hash()
			prevHash := node.PrevHash
			out += fmt.Sprintf("%s:%s:", hex.EncodeToString(hash[:]), hex.EncodeToString(prevHash[:]))
			if len(node.Transactions) > 0 {
				filenames := ""
				origins := ""
				for _, tx := range node.Transactions {
					if tx.File != nil {
						filenames += fmt.Sprintf("%s,", tx.File.Name)
					}
					if tx.Mapping != nil {
						origins += fmt.Sprintf("%s,", tx.Mapping.Identity)
					}
				}
				filenames = strings.TrimSuffix(filenames, ",")
				if len(filenames) > 0 {
					filenames = "Filenames=" + filenames
				}
				origins = strings.TrimSuffix(origins, ",")
				if len(origins) > 0 {
					if len(filenames) > 0 {
						filenames += ":"
					}
					origins = "Origins=" + origins
				}
				out += filenames + origins
			}
			return true
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
		printChain(&block)
	} else if height > bc.currentHeight {
		//
		// swap to longest fork
		//
		bc.mappings = sync.Map{}
		bc.claimedPubkey = *common.NewConcurrentSet()
		bc.pubKeyMapping = sync.Map{}
		bc.storeNewBlock(block)
		forEachBlockInFork(&block, func(node *common.Block) bool {
			bc.addNewMappings(node.Transactions)
			return true
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
		forEachBlockInFork(currentLastBlock, func(node *common.Block) bool {
			rewindedBlock++
			blockHeight[node.PrevHash] = rewindedBlock
			return true
		})
		// find the node where the fork happened
		forEachBlockInFork(&block, func(node *common.Block) bool {
			exists := false
			rewindedBlock, exists = blockHeight[node.PrevHash]
			return !exists
		})

		// Switch chain
		bc.longestChainLastBlock = block.Hash()

		// Remove invalid transactions
		removeInvalidTransaction()
		fmt.Printf("FORK-LONGER rewind %d blocks\n", rewindedBlock)
		printChain(&block)
	} else {
		createsFork := false
		bc.blocks.Range(func(_, blockRaw interface{}) bool {
			existentBlock := blockRaw.(common.Block)
			createsFork = existentBlock.PrevHash == block.PrevHash

			return !createsFork
		})
		bc.storeNewBlock(block)
		if createsFork {
			fmt.Printf("FORK-SHORTER %s\n", hex.EncodeToString(block.PrevHash[:]))
		}
	}

	bc.notifyMinerForChanges()
	return true
}

// Add new mappings. Should be called when a write lock on the Blockchain is taken !
func (bc *Blockchain) addNewMappings(txs []common.TxPublish) {
 	for _, tx := range txs {
		if tx.File != nil {
			bc.mappings.Store(tx.File.Name, hex.EncodeToString(tx.File.MetafileHash))
		}
		if tx.Mapping != nil {
			bc.claimedPubkey.Store(hex.EncodeToString(tx.Mapping.PublicKey))
			rsaPubkey, err := x509.ParsePKCS1PublicKey(tx.Mapping.PublicKey)
			if err != nil {
				fmt.Println("Error while parsing public key")
			}
			bc.pubKeyMapping.Store(tx.Mapping.Identity, rsaPubkey)
		}
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

// should be called when a RLock or Lock is taken on the bc
func (bc *Blockchain) getTransactions() []common.TxPublish {
	copied := make([]common.TxPublish, len(bc.pendingTransactions))
	copy(copied, bc.pendingTransactions)
	for index, tx := range copied {
		copied[index] = *tx.Clone()
	}

	return copied
}

func (bc *Blockchain) getPubKey (name string) (*rsa.PublicKey, bool) {
	rsaPubkey, found := bc.pubKeyMapping.Load(name)
	if !found {
		return nil, false
	}

	return rsaPubkey.(*rsa.PublicKey), true
}

func (bc *Blockchain) startMining(minedBlocks chan<- common.Block) {
	go func() {
		computingFirstBlock := true
		lastFoundBlockTime := time.Now()
		bc.Lock()
		block := common.Block{
			Transactions: bc.getTransactions(),
			PrevHash:     bc.longestChainLastBlock,
		}
		bc.Unlock()
		for {
			select {
			case _ = <-bc.changesNotifier:
				bc.Lock()
				block = common.Block{
					Transactions: bc.getTransactions(),
					PrevHash:     bc.longestChainLastBlock,
				}
				bc.Unlock()
			default:
				rand.Read(block.Nonce[:])
				if powIsCorrect(&block) && bc.AddBlock(block, true) {
					if computingFirstBlock {
						computingFirstBlock = false
						timer := time.NewTimer(common.FirstBlockPublicationDelay)
						<-timer.C
					} else {
						elapsedTime := time.Now().Sub(lastFoundBlockTime)
						timer := time.NewTimer(elapsedTime * 2)
						<-timer.C
					}
					minedBlocks <- block
					hash := block.Hash()
					lastFoundBlockTime = time.Now()
					fmt.Printf("FOUND-BLOCK %s\n", hex.EncodeToString(hash[:]))
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
