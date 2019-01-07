package gossiper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/yvgny/Peerster/common"
	"hash"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var DataCacheFolder = path.Join(common.HiddenStorageFolder, "chunks")

type DataManager struct {
	sync.RWMutex
	localFiles  sync.Map
	remoteFiles map[string]*RemoteFile
}

type LocalFile struct {
	Name       string
	Size       int64
	ChunkMap   []uint64
	MetaHash   string
	ChunkCount uint64
}

type RemoteFile struct {
	Name           string
	ChunkCount     uint64
	ChunkLocations map[uint64][]string
}

func NewDataManager() (*DataManager, error) {
	if _, err := os.Stat(DataCacheFolder); os.IsNotExist(err) {
		err = os.Mkdir(DataCacheFolder, os.ModePerm)
		if err != nil {
			return nil, err
		}
	}
	return &DataManager{
		localFiles:  sync.Map{},
		remoteFiles: make(map[string]*RemoteFile),
	}, nil
}

func (dm *DataManager) addChunkLocation(metafileHash, filename string, chunkNumbers []uint64, totalChunksCount uint64, host string) {
	dm.Lock()
	defer dm.Unlock()

	// Load remote file or create a new one
	remoteFile, ok := dm.remoteFiles[metafileHash]
	if !ok {
		remoteFile = &RemoteFile{
			Name:           filename,
			ChunkCount:     totalChunksCount,
			ChunkLocations: make(map[uint64][]string),
		}
		dm.remoteFiles[metafileHash] = remoteFile
	}

	// Add the new chunks
	for _, chunkNumber := range chunkNumbers {
		if list, ok := remoteFile.ChunkLocations[chunkNumber]; !ok {
			list = []string{host}
			remoteFile.ChunkLocations[chunkNumber] = list
		} else if !common.Contains(host, remoteFile.ChunkLocations[chunkNumber]) {
			remoteFile.ChunkLocations[chunkNumber] = append(list, host)
		}
	}
}

func (dm *DataManager) getRandomChunkLocation(metafileHash string, chunkNumber uint64) (string, bool) {
	dm.RLock()
	defer dm.RUnlock()

	remoteFile, ok := dm.remoteFiles[metafileHash]
	if !ok {
		return "", false
	}

	list, ok := remoteFile.ChunkLocations[chunkNumber]
	if !ok {
		return "", false
	}

	return list[rand.Intn(len(list))], true
}

// return every matched file, mapping the filename to their metafile hash
func (dm *DataManager) getAllRemoteMatches() map[string]string {
	dm.RLock()
	defer dm.RUnlock()

	result := make(map[string]string)

	for key, metadata := range dm.remoteFiles {
		if dm.remoteFileIsMatch(key) {
			result[metadata.Name] = key
		}
	}

	return result
}

func (dm *DataManager) remoteFileIsMatch(metafileHash string) bool {
	return dm.numberOfMatch(metafileHash) > 0
}

func (dm *DataManager) numberOfMatch(metafileHash string) uint {
	dm.RLock()
	defer dm.RUnlock()
	remoteFile, ok := dm.remoteFiles[metafileHash]
	if !ok {
		return 0
	}

	if remoteFile.ChunkCount != uint64(len(remoteFile.ChunkLocations)) {
		return 0
	}

	var min uint = math.MaxInt32
	for _, locations := range remoteFile.ChunkLocations {
		length := uint(len(locations))
		if length < min {
			min = length
		}
	}

	return min
}

func (dm *DataManager) removeLocalFile(metafileHash string) error {
	hash, err := hex.DecodeString(metafileHash)
	if err != nil {
		return err
	}
	metafile, err := dm.getLocalData(hash)
	if err != nil {
		return err
	}

	if len(metafile)%sha256.Size != 0 {
		return errors.New("cannot remove local file: invalid metafile length")
	}

	for i := 0; i < len(metafile); i += sha256.Size {
		metahash := metafile[i : i+sha256.Size]
		chunkHex := hex.EncodeToString(metahash)
		_ = os.Remove(filepath.Join(DataCacheFolder, chunkHex))
	}

	_ = os.Remove(filepath.Join(DataCacheFolder, metafileHash))

	dm.localFiles.Delete(metafileHash)

	return nil
}

// Index a new file and returns the hash of its meta file. If a key is given, the chunks are encrypted
// with symmetric encryption. Otherwise a nil value can be given and chunks a stored in plaintext
func (dm *DataManager) addLocalFile(path string, key *[32]byte) (*LocalFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, DefaultChunkSize)

	metafile := make([]byte, 0)

	chunkMap := make([]uint64, 0)
	chunkCount := uint64(0)
	for {
		bytesread, err := file.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}

			break
		}

		dataChunk := buffer[:bytesread]
		if key != nil {
			dataChunk, err = common.EncryptChunk(dataChunk, *key)
			if err != nil {
				return nil, err
			}
		}
		chunkHash := sha256.Sum256(dataChunk)
		filename := hex.EncodeToString(chunkHash[:])
		err = ioutil.WriteFile(filepath.Join(DataCacheFolder, filename), dataChunk, os.ModePerm)
		if err != nil {
			return nil, err
		}

		// Add this chunk to the available chunk
		chunkCount++
		chunkMap = append(chunkMap, chunkCount)

		metafile = append(metafile, chunkHash[:]...)
	}
	metafileHash := sha256.Sum256(metafile)
	filename := hex.EncodeToString(metafileHash[:])
	err = ioutil.WriteFile(filepath.Join(DataCacheFolder, filename), metafile, os.ModePerm)
	if err != nil {
		return nil, err
	}

	stats, _ := file.Stat()
	lf := dm.addLocalRecord(filename, stats.Name(), chunkMap, chunkCount, stats.Size())

	return lf, nil
}

func (dm *DataManager) addLocalData(data, hash []byte) error {
	hashBytes := sha256.Sum256(data)
	hash1 := hex.EncodeToString(hash)
	hash2 := hex.EncodeToString(hashBytes[:])

	if hash1 != hash2 {
		return errors.New("provided hash does not match with hash computed from data")
	}

	err := ioutil.WriteFile(filepath.Join(DataCacheFolder, hash1), data, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func (dm *DataManager) addLocalRecord(hash, filename string, chunckMap []uint64, chunkCount uint64, size int64) *LocalFile {
	md := LocalFile{
		Name:       filename,
		ChunkMap:   chunckMap,
		MetaHash:   hash,
		ChunkCount: chunkCount,
	}

	dm.localFiles.Store(hash, md)

	return &md
}

func (dm *DataManager) getLocalRecord(hash string) (*LocalFile, error) {
	rawData, ok := dm.localFiles.Load(hash)
	if !ok {
		return nil, errors.New("cannot find requested record")
	}
	md := rawData.(LocalFile)

	return &md, nil
}

func (dm *DataManager) getLocalData(hash []byte) ([]byte, error) {
	hashStr := hex.EncodeToString(hash)
	rawData, err := ioutil.ReadFile(filepath.Join(DataCacheFolder, hashStr))
	if err != nil {
		return nil, err
	}

	return rawData, nil
}

func (dm *DataManager) HashChunksOfLocalFile(metafileHash []byte, chunkMap []uint64, hashWriter hash.Hash) ([]byte, error) {
	metafile, err := dm.getLocalData(metafileHash)
	if err != nil {
		return nil, err
	}
	for _, chunkNum := range chunkMap {
		currHash := metafile[(chunkNum-1)*sha256.Size : chunkNum*sha256.Size]
		data, err := dm.getLocalData(currHash)
		if err != nil {
			return nil, err
		}
		hashWriter.Write(data)
	}

	return hashWriter.Sum(nil), nil
}

func (dm *DataManager) SearchLocalFile(keywords []string) []*common.SearchResult {
	results := make([]*common.SearchResult, 0)
	hashList := make([]string, 0)
	filter := CreateFilterFromKeywords(keywords)
	dm.localFiles.Range(func(hash, metadataRaw interface{}) bool {
		metadata := metadataRaw.(LocalFile)
		fmt.Printf("%s is in the recorded files\n", metadata.Name)
		if filter(metadata.Name) {
			hashByte, err := hex.DecodeString(metadata.MetaHash)
			if err != nil || common.Contains(metadata.MetaHash, hashList) {
				return true
			}
			sr := common.SearchResult{
				FileName:     metadata.Name,
				MetafileHash: hashByte,
				ChunkMap:     metadata.ChunkMap,
				ChunkCount:   metadata.ChunkCount,
			}
			hashList = append(hashList, metadata.MetaHash)
			results = append(results, &sr)
		}

		return true
	})
	fmt.Printf("results = %v\n", results)
	return results
}

func CreateFilterFromKeywords(keywords []string) func(string) bool {
	return func(s string) (valid bool) {
		for _, keyword := range keywords {
			valid = valid || strings.Contains(s, keyword)
		}

		return
	}
}
