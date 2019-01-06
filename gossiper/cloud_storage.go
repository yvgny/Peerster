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
	"path/filepath"
	"sync"
	"time"
)

var DiskStorageLocation = path.Join(common.HiddenStorageFolder, "cloud")
var mappingsLoc = path.Join(DiskStorageLocation, "files.json")

type CloudStorage struct {
	sync.RWMutex
	mappings map[string]LocalFile // Map (filename -> LocalFile)
}

func CreateNewCloudStorage() *CloudStorage {
	return &CloudStorage{
		mappings: make(map[string]LocalFile),
	}
}

func LoadCloudStorageFromDisk() (*CloudStorage, error) {
	bytes, err := ioutil.ReadFile(mappingsLoc)
	if err != nil {
		return nil, err
	}
	var cs map[string]LocalFile
	err = json.Unmarshal(bytes, &cs)
	if err != nil {
		return nil, errors.New("cannot load CloudStorage: malformed file")
	}
	return &CloudStorage{
		mappings: cs,
	}, nil
}

func (cs *CloudStorage) GetAllMappings() map[string]LocalFile {
	cs.RLock()
	defer cs.RUnlock()
	mapCopy := make(map[string]LocalFile)
	for key, value := range cs.mappings {
		mapCopy[key] = value
	}

	return mapCopy
}

func (cs *CloudStorage) GetInfoOfFile(filename string) (*LocalFile, bool) {
	cs.RLock()
	defer cs.RUnlock()
	val, ok := cs.mappings[filename]
	return &val, ok
}

func (cs *CloudStorage) Exists(filename string) bool {
	cs.RLock()
	defer cs.RUnlock()
	_, ok := cs.mappings[filename]
	return ok
}

func (cs *CloudStorage) AddMapping(filename string, hash *LocalFile) error {
	cs.Lock()
	defer cs.Unlock()
	oldValue, oldValueExists := cs.mappings[filename]
	cs.mappings[filename] = *hash
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

func (g *Gossiper) DownloadFileFromCloud(filename string, blockchain *Blockchain) error {
	fileInfo, _ := g.cloudStorage.GetInfoOfFile(filename)

	var metaHash [32]byte
	metaHashSlice, err := hex.DecodeString(fileInfo.MetaHash)
	if err != nil {
		return err
	}
	copy(metaHash[:], metaHashSlice)

	nonce := generateNonce()

	if err != nil {
		return err
	}

	request := common.GossipPacket{
		UploadedFileRequest: &common.UploadedFileRequest{
			Origin:   g.name,
			Nonce:    nonce,
			MetaHash: metaHash,
		},
	}
	common.BroadcastMessage(g.peers.Elements(), request, nil, g.gossipConn)

	channel := make(chan *common.UploadedFileReply)
	g.waitCloudRequest.Store(fileInfo, channel)
	for {
		timer := time.NewTimer(common.CloudSearchTimeout)
		select {
		case reply := <-channel:
			pubkey, found := blockchain.getPubKey(reply.Origin)
			if !found {
				continue
			}
			if reply.VerifySignature(pubkey, nonce) {
				continue
			}
			g.data.addChunkLocation(fileInfo.MetaHash, filename, reply.OwnedChunks, fileInfo.ChunkCount, reply.Origin)
			if g.data.remoteFileIsMatch(fileInfo.MetaHash) {
				err = g.downloadFile("", metaHash[:], filename, &g.keychain.SymmetricKey)
				if err != nil {
					return errors.New("could not download file: " + err.Error())
				}
				_ = g.data.removeLocalFile(fileInfo.MetaHash)
				g.waitCloudRequest.Delete(fileInfo)
				return nil
			}
		case <-timer.C:
			g.waitCloudRequest.Delete(fileInfo)
			return errors.New("could not download file: peer replies timeout")
		}
	}
}

func (g *Gossiper) UploadFileToCloud(filename string, blockchain *Blockchain) (*LocalFile, error) {
	//TODO Choose right path for files to upload
	fileInfo, err := g.data.addLocalFile(filepath.Join(common.CloudFilesUploadFolder, filename), &g.keychain.SymmetricKey)
	if err != nil {
		return nil, err
	}
	metaHashStr := fileInfo.MetaHash
	var metaHash [32]byte
	metaHashSlice, err := hex.DecodeString(metaHashStr)
	if err != nil {
		return nil, err
	}
	copy(metaHash[:], metaHashSlice)

	metaFile, err := g.data.getLocalData(metaHashSlice)
	if err != nil {
		return nil, err
	}

	nonce := generateNonce()

	message := common.FileUploadMessage{
		Origin:         g.name,
		UploadedChunks: []uint64{},
		Nonce:          nonce,
		MetaHash:       metaHash,
		MetaFile:       metaFile,
		HopLimit:       common.BlockBroadcastHopLimit,
	}
	gossipPacket := common.GossipPacket{
		FileUploadMessage: &message,
	}
	common.BroadcastMessage(g.peers.Elements(), gossipPacket, nil, g.gossipConn)

	localFile, err := g.data.getLocalRecord(metaHashStr)
	if err != nil {
		return nil, err
	}
	channel := make(chan *common.FileUploadAck)
	g.waitCloudStorage.Store(metaHashStr, channel)
	foundFullMatch := false
	//TODO : Determine termination condition
	timer := time.NewTicker(time.Second * 10)
	for {
		select {
		case ack := <-channel:
			pubKey, found := blockchain.getPubKey(ack.Origin)
			if !found {
				continue
			}
			chunksHash, err := g.data.HashChunksOfLocalFile(metaHashSlice, ack.UploadedChunks, sha256.New())
			if err != nil {
				continue
			}
			if !ack.VerifySignature(pubKey, nonce, chunksHash) {
				continue
			}
			g.data.addChunkLocation(metaHashStr, filename, ack.UploadedChunks, localFile.ChunkCount, ack.Origin)
			if g.data.remoteFileIsMatch(metaHashStr) {
				foundFullMatch = true
			}
		case <-timer.C:
			g.waitCloudStorage.Delete(metaHashStr)
			close(channel)
			if !foundFullMatch {
				return nil, errors.New("the file could not be entirely uploaded among other peers, try again")
			}
			return fileInfo, nil
		}
	}
}

func (g *Gossiper) HandleClientCloudRequest(filename string, blockchain *Blockchain) error {
	if exists := g.cloudStorage.Exists(filename); exists {
		err := g.DownloadFileFromCloud(filename, blockchain)
		if err != nil {
			return errors.New(fmt.Sprintf("Cannot download file from cloud: %s\n", err.Error()))
		}
	} else {
		hash, err := g.UploadFileToCloud(filename, blockchain)
		if err != nil {
			return errors.New(fmt.Sprintf("Cannot upload file to cloud: %s\n", err.Error()))
		}
		err = g.cloudStorage.AddMapping(filename, hash)
		if err != nil {
			return errors.New(fmt.Sprintf("Cannot save cloud record on disk: %s\n", err.Error()))
		}
	}

	return nil
}

func generateNonce() [32]byte {
	var nonce [32]byte
	_, _ = rand.Read(nonce[:])

	return nonce
}
