package common

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
)

const RsaKeyBitLength = 2048

var DiskStorageLocation = path.Join(HiddenStorageFolder, "keys")
var asymKeyLoc = path.Join(DiskStorageLocation, "asym.pem")
var symKeyLoc = path.Join(DiskStorageLocation, "sym.pem")

type KeyStorage struct {
	AsymmetricPrivKey *rsa.PrivateKey
	SymmetricKey      [32]byte
}

func GenerateNewKeyStorage() (*KeyStorage, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, RsaKeyBitLength)
	if err != nil {
		return nil, err
	}
	var symmKey [32]byte
	_, err = rand.Read(symmKey[:])
	if err != nil {
		return nil, err
	}

	ks := &KeyStorage{
		AsymmetricPrivKey: privKey,
		SymmetricKey:      symmKey,
	}

	return ks, nil
}

func LoadKeyStorageFromDisk() (*KeyStorage, error) {
	pemdata, err := ioutil.ReadFile(asymKeyLoc)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cannot parse public key from %s: %s", asymKeyLoc, err.Error()))
	}
	block, _ := pem.Decode(pemdata)
	if block == nil {
		return nil, errors.New(fmt.Sprintf("cannot read KeyStorage from %s: no pem format found", asymKeyLoc))
	}

	asymKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cannot parse public key from %s: %s", asymKeyLoc, err.Error()))
	}

	symBytes, err := ioutil.ReadFile(symKeyLoc)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cannot parse symmetric key from %s: %s", symKeyLoc, err.Error()))
	}

	ks := KeyStorage{
		AsymmetricPrivKey: asymKey,
	}

	_, err = base64.StdEncoding.Decode(ks.SymmetricKey[:], symBytes)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cannot parse public key from %s: %s", asymKeyLoc, err.Error()))
	}

	return &ks, nil
}

func (ks *KeyStorage) SaveKeyStorageOnDisk() error {
	pemdata := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(ks.AsymmetricPrivKey),
	})
	err := ioutil.WriteFile(asymKeyLoc, pemdata, os.ModePerm)
	if err != nil {
		return err
	}

	symKeyMarshal := make([]byte, base64.StdEncoding.EncodedLen(len(ks.SymmetricKey)))
	base64.StdEncoding.Encode(symKeyMarshal, ks.SymmetricKey[:])
	err = ioutil.WriteFile(symKeyLoc, symKeyMarshal, os.ModePerm)
	if err != nil {
		_ = os.Remove(asymKeyLoc)
		return err
	}

	return nil
}
