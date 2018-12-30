package gossiper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sync"
	"time"
)

var DiskStorageLocation = path.Join(common.HiddenStorageFolder, "cloud")
var mappingsLoc = path.Join(DiskStorageLocation, "files.json")

type CloudStorage struct {
	sync.RWMutex
	mappings map[string]string // Map (filename -> metahash),with metahash in hex
}

func CreateNewCloudStorage() *CloudStorage {
	return &CloudStorage{
		mappings: make(map[string]string),
	}
}

func LoadCloudStorageFromDisk() (*CloudStorage, error) {
	bytes, err := ioutil.ReadFile(mappingsLoc)
	if err != nil {
		return nil, err
	}
	var csRaw0 interface{}
	err = json.Unmarshal(bytes, &csRaw0)
	if err != nil {
		return nil, errors.New("cannot load CloudStorage: malformed file")
	}
	csRaw, ok := csRaw0.(map[string]interface{})
	if !ok {
		return nil, errors.New("cannot load CloudStorage: malformed file")
	}
	cs := make(map[string]string)
	for key, value := range csRaw {
		hash, valid := value.(string)
		if !valid {
			return nil, errors.New("cannot load CloudStorage: malformed file")
		}
		cs[key] = hash
	}

	return &CloudStorage{
		mappings: cs,
	}, nil
}

func (cs *CloudStorage) GetHashOfFile(filename string) (string, bool) {
	cs.RLock()
	defer cs.RUnlock()
	val, ok := cs.mappings[filename]
	return val, ok
}

func (cs *CloudStorage) Exists(filename string) bool {
	cs.RLock()
	defer cs.RUnlock()
	_, ok := cs.mappings[filename]
	return ok
}

func (cs *CloudStorage) AddMapping(filename, hash string) error {
	cs.Lock()
	defer cs.Unlock()
	oldValue, oldValueExists := cs.mappings[filename]
	cs.mappings[filename] = hash
	err := cs.saveCloudStorageOnDiskWithoutLock()
	if err != nil {
		delete(cs.mappings, filename)
		if oldValueExists {
			cs.mappings[filename] = oldValue
		}
		return err
	}

	return nil
}

func (cs *CloudStorage) saveCloudStorageOnDiskWithoutLock() error {
	if _, err := os.Stat(DiskStorageLocation); os.IsNotExist(err) {
		err = os.Mkdir(DiskStorageLocation, os.ModePerm)
		if err != nil {
			return err
		}
	}

	bytes, err := json.Marshal(cs.mappings)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(mappingsLoc, bytes, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func (g *Gossiper) DownloadFileFromCloud(filename string) error {
	metaHashStr, _ := g.cloudStorage.GetHashOfFile(filename)

	var metaHash [32]byte
	metaHashSlice, err := hex.DecodeString(metaHashStr)
	if err != nil {
		return err
	}
	copy(metaHash[:], metaHashSlice)

	nonce, err := generateNonce()
	if err != nil {
		return err
	}

	localFile, err := g.data.getLocalRecord(metaHashStr)
	if err != nil {
		return err
	}

	request := common.GossipPacket{ UploadedFileRequest:&common.UploadedFileRequest{ Origin:g.name, Nonce:nonce, MetaHash:metaHash }}
	relayAddr := g.gossipAddress.String()
	common.BroadcastMessage(g.peers.Elements(), request, &relayAddr, g.gossipConn)

	channel := make(chan *common.UploadedFileReply)
	g.waitCloudRequest.Store(metaHashStr, channel)
	go func() {
		for {
			select {
			case reply := <- channel:
				//TODO : get public key, to verify signature
				if reply.VerifySignature(nil, nonce) {
					continue
				}
				g.data.addChunkLocation(metaHashStr, filename, reply.OwnedChunks, localFile.ChunkCount, reply.Origin)
				if g.data.remoteFileIsMatch(metaHashStr) {
					err = g.downloadFile("", metaHash[:], filename)
					if err != nil {
						fmt.Println("Could not download file : " + err.Error())
					}
					return
				}
			}
		}
	}()

	return nil
}

func (g *Gossiper) UploadFileToCloud(filename string) (string, error) {
	//TODO Choose right path for files to upload
	metaHashSlice, err := g.data.addLocalFile(filename)
	if err != nil {
		return "", err
	}
	metaHashStr := hex.EncodeToString(metaHashSlice)
	var metaHash [32]byte
	copy(metaHash[:], metaHashSlice)

	metaFile, err := g.data.getLocalData(metaHashSlice)
	if err != nil {
		return "", err
	}

	nonce, err := generateNonce()
	if err != nil {
		return "", err
	}

	message := common.FileUploadMessage{ Origin:g.name, UploadedChunks:[]uint64{}, Nonce:nonce, MetaHash:metaHash,
		MetaFile: metaFile, HopLimit:common.BlockBroadcastHopLimit }
	gossipPacket := common.GossipPacket{ FileUploadMessage:&message }
	relayAddr := g.gossipAddress.String()
	common.BroadcastMessage(g.peers.Elements(), gossipPacket, &relayAddr, g.gossipConn)

	localFile, err := g.data.getLocalRecord(metaHashStr)
	if err != nil {
		return "", err
	}
	channel := make(chan *common.FileUploadAck)
	g.waitCloudStorage.Store(metaHashStr, channel)
	foundFullMatch := false
	go func() {
		//TODO : Determine termination condition
		timer := time.NewTicker(time.Second * 10)
		for {
			select {
			case ack := <- channel:
				//TODO : get public key, to verify signature, also get chunks
				if ack.VerifySignature(nil, nonce, [][]byte {}) {
					continue
				}
				g.data.addChunkLocation(metaHashStr, filename, ack.UploadedChunks, localFile.ChunkCount, ack.Origin)
				if g.data.remoteFileIsMatch(metaHashStr) {
					foundFullMatch = true
				}
			case <- timer.C:
				g.waitCloudStorage.Delete(metaHashStr)
				close(channel)
				if !foundFullMatch {
					fmt.Println("The file could not be entirely uploaded among other peers, try again.")
				}
				return
			}
		}
	}()

	return metaHashStr, nil
}

func generateNonce() ([32]byte, error) {
	var nonce [32]byte
	randNonce := make([]byte, sha256.Size)
	_, err := rand.Read(randNonce)
	if err != nil {
		return nonce, err
	}
	copy(nonce[:], randNonce)

	return nonce, nil
}