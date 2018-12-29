package common

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
)

// Returns the base64-encoded version of the encrypted string
func EncryptText(text string, key *rsa.PublicKey) (string, error) {
	encrypt, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, []byte(text), nil)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(encrypt), nil
}

// Takes a base64-encoded cipher an returns the decoded string
func DecryptText(text string, key *rsa.PrivateKey) (string, error) {
	encrypted, err := base64.RawStdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	decrypted, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, encrypted, nil)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// Encrypt a chunk with a symmetric key
func EncryptChunk(chunk []byte, key [32]byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aesgsm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aesgsm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, err
	}
	ciphertext := aesgsm.Seal(nil, nonce, chunk, nil)
	final := make([]byte, aesgsm.NonceSize()+len(ciphertext))
	copy(final[:len(nonce)], nonce)
	copy(final[len(nonce):], ciphertext)

	return final, nil
}

// Decrypt a chunk using a symmetric key
func DecryptChunk(ciphertext []byte, key [32]byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aesgsm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	chunk, err := aesgsm.Open(nil, ciphertext[:aesgsm.NonceSize()], ciphertext[aesgsm.NonceSize():], nil)
	if err != nil {
		return nil, err
	}

	return chunk, nil
}

func (rm *RumorMessage) Sign(key *rsa.PrivateKey) {
	hash := rm.Hash()
	signature, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, hash[:], nil)
	rm.Signature = signature
}

func (rm *RumorMessage) VerifySignature(key *rsa.PublicKey) bool {
	hash := rm.Hash()
	err := rsa.VerifyPSS(key, crypto.SHA256, hash[:], rm.Signature, nil)
	return err == nil
}

func (pm *PrivateMessage) Sign(key *rsa.PrivateKey) {
	hash := pm.Hash()
	signature, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, hash[:], nil)
	pm.Signature = signature
}

func (pm *PrivateMessage) VerifySignature(key *rsa.PublicKey) bool {
	hash := pm.Hash()
	err := rsa.VerifyPSS(key, crypto.SHA256, hash[:], pm.Signature, nil)
	return err == nil
}

func (id *IdentityPKeyMapping) Sign(key *rsa.PrivateKey) {
	hash := id.Hash()
	signature, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, hash[:], nil)
	id.Signature = signature
}

func (id *IdentityPKeyMapping) VerifySignature() bool {
	hash := id.Hash()
	key, err := x509.ParsePKCS1PublicKey(id.PublicKey)
	if err != nil {
		return false
	}
	err = rsa.VerifyPSS(key, crypto.SHA256, hash[:], id.Signature, nil)
	return err == nil
}

// The nonce from FileUploadMessage should be given. The array of chunks is the
// chunks selected in UploadedChunks
func (fua *FileUploadAck) Sign(key *rsa.PrivateKey, nonce [32]byte, chunks [][]byte) {
	if len(chunks) != len(fua.UploadedChunks) {
		return
	}
	hash := fua.Hash(chunks, nonce)
	fua.Signature, _ = rsa.SignPSS(rand.Reader, key, crypto.SHA256, hash[:], nil)
}

// The nonce from FileUploadMessage should be given. The array of chunks is the
// chunks selected in UploadedChunks
func (fua *FileUploadAck) VerifySignature(key *rsa.PublicKey, nonce [32]byte, chunks [][]byte) bool {
	if len(chunks) != len(fua.UploadedChunks) {
		return false
	}
	hash := fua.Hash(chunks, nonce)
	err := rsa.VerifyPSS(key, crypto.SHA256, hash[:], fua.Signature, nil)
	return err == nil
}

// The nonce from UploadedFileRequest should be given
func (ufr *UploadedFileReply) Sign(key *rsa.PrivateKey, nonce [32]byte) {
	hash := ufr.Hash(nonce)
	ufr.Signature, _ = rsa.SignPSS(rand.Reader, key, crypto.SHA256, hash[:], nil)
}

// The nonce from UploadedFileRequest should be given
func (ufr *UploadedFileReply) VerifySignature(key *rsa.PublicKey, nonce [32]byte) bool {
	hash := ufr.Hash(nonce)
	err := rsa.VerifyPSS(key, crypto.SHA256, hash[:], ufr.Signature, nil)
	return err == nil
}
