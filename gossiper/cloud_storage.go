package gossiper

import (
	"encoding/json"
	"errors"
	"github.com/yvgny/Peerster/common"
	"io/ioutil"
	"os"
	"path"
	"sync"
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

func (cs *CloudStorage) GetALlMappings() map[string]string {
	cs.RLock()
	defer cs.RUnlock()
	mapCopy := make(map[string]string)
	for key, value := range cs.mappings {
		mapCopy[key] = value
	}

	return mapCopy
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

func (g *Gossiper) DownloadFileFromCloud(metafileHash string) error {
	// TODO
	return nil
}

func (g *Gossiper) UploadFileToCloud(filename string) (string, error) {
	// TODO
	return "", nil
}
