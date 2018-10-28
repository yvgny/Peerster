package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
)

type DataManager struct {
	data sync.Map
}

func NewDataManager() *DataManager {
	return &DataManager{
		data: sync.Map{},
	}
}

// Index a new file and returns the has of its meta file
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
		copiedData := make([]byte, len(dataChunk))
		copy(copiedData, dataChunk)
		dm.data.Store(hex.EncodeToString(chunkHash[:]), copiedData)
		metafile = append(metafile, chunkHash[:]...)
	}
	metafileHash := sha256.Sum256(metafile)
	dm.data.Store(hex.EncodeToString(metafileHash[:]), metafile)

	return metafileHash[:], nil
}

func (dm *DataManager) addData(data, hash []byte) error {
	hashBytes := sha256.Sum256(data)
	hash1 := hex.EncodeToString(hash)
	hash2 := hex.EncodeToString(hashBytes[:])

	if hash1 != hash2 {
		return errors.New("provided hash does not match with hash computed from data")
	}

	copied := make([]byte, len(data))
	copy(copied, data)

	dm.data.Store(hash1, copied)

	return nil
}

func (dm *DataManager) getData(hash []byte) ([]byte, bool) {
	hashStr := hex.EncodeToString(hash)
	rawData, ok := dm.data.Load(hashStr)
	if !ok {
		return nil, false
	}
	data := rawData.([]byte)
	copied := make([]byte, len(data))
	copy(copied, data)

	return copied, true
}
