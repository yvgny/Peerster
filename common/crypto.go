package common

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
)

func EncryptText(text string, key *rsa.PublicKey) string {
	encrypt, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, []byte(text), nil)
	return base64.StdEncoding.EncodeToString(encrypt)
}

func DecryptText(text string, key *rsa.PrivateKey) (string, error) {
	encrypted, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	decrypted, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, encrypted, nil)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

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
	final := make([]byte, len(nonce)+len(ciphertext))
	copy(final[:len(nonce)], nonce)
	copy(final[len(nonce):], ciphertext)

	return final, nil
}

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
	signature, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, rm.Hash()[:], nil)
	rm.Signature = signature
}

func (rm *RumorMessage) VerifySignature(key *rsa.PublicKey) bool {
	err := rsa.VerifyPSS(key, crypto.SHA256, rm.Hash()[:], rm.Signature, nil)
	return err == nil
}

func (pm *PrivateMessage) Sign(key *rsa.PrivateKey) {
	signature, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, pm.Hash()[:], nil)
	pm.Signature = signature
}

func (pm *PrivateMessage) VerifySignature(key *rsa.PublicKey) bool {
	err := rsa.VerifyPSS(key, crypto.SHA256, pm.Hash()[:], pm.Signature, nil)
	return err == nil
}

func (id *IdentityPKeyMapping) Sign(key *rsa.PrivateKey) {
	signature, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, id.Hash()[:], nil)
	id.Signature = signature
}

func (id *IdentityPKeyMapping) VerifySignature(key *rsa.PublicKey) bool {
	err := rsa.VerifyPSS(key, crypto.SHA256, id.Hash()[:], id.Signature, nil)
	return err == nil
}

func (fua *FileUploadAck) Sign(key *rsa.PrivateKey, nonce [32]byte, chunks [][]byte) {
	if len(chunks) != len(fua.UploadedChunks) {
		return
	}
	fua.Signature, _ = rsa.SignPSS(rand.Reader, key, crypto.SHA256, fua.Hash(chunks, nonce)[:], nil)
}

func (fua *FileUploadAck) VerifySignature(key *rsa.PublicKey, nonce [32]byte, chunks [][]byte) bool {
	if len(chunks) != len(fua.UploadedChunks) {
		return false
	}
	err := rsa.VerifyPSS(key, crypto.SHA256, fua.Hash(chunks, nonce)[:], fua.Signature, nil)
	return err == nil
}
