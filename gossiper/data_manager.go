package gossiper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/yvgny/Peerster/common"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const DataCacheFolder = ".peerster"

type DataManager struct {
	records sync.Map
}

type FileMetaData struct {
	Name       string
	ChunkMap   []uint64
	MetaHash   string
	ChunkCount uint64
}

func NewDataManager() *DataManager {
	if _, err := os.Stat(DataCacheFolder); os.IsNotExist(err) {
		os.Mkdir(DataCacheFolder, os.ModePerm)
	}
	return &DataManager{
		records: sync.Map{},
	}
}

// Index a new file and returns the hash of its meta file
func (dm *DataManager) addFile(path string) ([]byte, error) {
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
	chunkCount := uint64(1)
	for {
		bytesread, err := file.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}

			break
		}

		dataChunk := buffer[:bytesread]
		chunkHash := sha256.Sum256(dataChunk)
		filename := hex.EncodeToString(chunkHash[:])
		err = ioutil.WriteFile(filepath.Join(DataCacheFolder, filename), dataChunk, os.ModePerm)
		if err != nil {
			return nil, err
		}

		// Add this chunk to the available chunk
		chunkMap = append(chunkMap, chunkCount)
		chunkCount++

		metafile = append(metafile, chunkHash[:]...)
	}
	metafileHash := sha256.Sum256(metafile)
	filename := hex.EncodeToString(metafileHash[:])
	err = ioutil.WriteFile(filepath.Join(DataCacheFolder, filename), metafile, os.ModePerm)
	if err != nil {
		return nil, err
	}

	dm.addRecord(filename, file.Name(), chunkMap, chunkCount)

	return metafileHash[:], nil
}

func (dm *DataManager) addData(data, hash []byte) error {
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

func (dm *DataManager) addRecord(hash, filename string, chunckMap []uint64, chunkCount uint64) {
	md := FileMetaData{
		Name:       filename,
		ChunkMap:   chunckMap,
		MetaHash:   hash,
		ChunkCount: chunkCount,
	}

	dm.records.Store(hash, md)
}

func (dm *DataManager) getRecord(hash string) (*FileMetaData, error) {
	rawData, ok := dm.records.Load(hash)
	if !ok {
		return nil, errors.New("cannot find requested record")
	}
	md := rawData.(FileMetaData)

	return &md, nil
}

func (dm *DataManager) getData(hash []byte) ([]byte, error) {
	hashStr := hex.EncodeToString(hash)
	rawData, err := ioutil.ReadFile(filepath.Join(DataCacheFolder, hashStr))
	if err != nil {
		return nil, err
	}

	return rawData, nil
}

func (dm *DataManager) SearchFile(keywords []string) []*common.SearchResult {
	results := make([]*common.SearchResult, 0)
	dm.records.Range(func(hash, metadataRaw interface{}) bool {
		metadata := metadataRaw.(FileMetaData)
		for _, keyword := range keywords {
			if strings.Contains(metadata.Name, keyword) {
				hashByte, err := hex.DecodeString(metadata.MetaHash)
				if err != nil {
					return true
				}
				sr := common.SearchResult{
					FileName:     metadata.Name,
					MetafileHash: hashByte,
					ChunkMap:     metadata.ChunkMap,
				}
				results = append(results, &sr)
			}
		}

		return true
	})

	return results
}
