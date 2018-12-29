package common

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"github.com/dedis/protobuf"
	"net"
)

const LocalAddress = "127.0.0.1"

// Basic message type
type SimpleMessage struct {
	OriginalName string
	Contents     string
}

type RumorMessage struct {
	Origin    string
	ID        uint32
	Text      string
	Signature Signature
}

type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string // base64 encoding of encrypted msg
	Destination string
	HopLimit    uint32
	Signature   Signature
}

type PeerStatus struct {
	Identifier string
	NextID     uint32
}

type StatusPacket struct {
	Want []PeerStatus
}

type FileIndexPacket struct {
	Filename string
}

type FileDownloadPacket struct {
	User      string
	HashValue []byte
	Filename  string
}

type CloudIndexPacket struct {
	Filename string
}

type CloudDownloadPacket struct {
	Filename string
}

type DataRequest struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
}

type DataReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
	Data        []byte
}

type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}

type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}

type SearchResult struct {
	FileName     string
	MetafileHash []byte
	ChunkMap     []uint64
	ChunkCount   uint64
}

type TxPublish struct {
	File     *File
	HopLimit uint32
	Mapping  *IdentityPKeyMapping
}

type BlockPublish struct {
	Block    Block
	HopLimit uint32
}

type File struct {
	Name         string
	Size         int64
	MetafileHash []byte
}

type Block struct {
	PrevHash     [32]byte
	Nonce        [32]byte
	Transactions []TxPublish
}

// Packet exchanged between peers
type GossipPacket struct {
	Simple              *SimpleMessage
	Rumor               *RumorMessage
	Status              *StatusPacket
	Private             *PrivateMessage
	DataRequest         *DataRequest
	DataReply           *DataReply
	SearchRequest       *SearchRequest
	SearchReply         *SearchReply
	TxPublish           *TxPublish
	BlockPublish        *BlockPublish
	FileUploadAck       *FileUploadAck
	FileUploadMessage   *FileUploadMessage
	UploadedFileRequest *UploadedFileRequest
	UploadedFileReply   *UploadedFileReply
}

// Packet exchanged with the client
type ClientPacket struct {
	GossipPacket
	FileIndex           *FileIndexPacket
	FileDownload        *FileDownloadPacket
	CloudIndexPacket    *CloudIndexPacket
	CloudDownloadPacket *CloudDownloadPacket
}

type Signature []byte

type IdentityPKeyMapping struct {
	Identity  string
	PublicKey []byte
	Signature Signature
}

func CreateNewIdendityPKeyMapping(identity string, key *rsa.PrivateKey) *IdentityPKeyMapping {
	id := IdentityPKeyMapping{
		Identity:  identity,
		PublicKey: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	}

	id.Sign(key)

	return &id
}

type FileUploadMessage struct {
	MetaHash       [32]byte
	MetaFile       []byte
	HopLimit       uint64
	UploadedChunks []uint64
	Nonce          [32]byte
}

type FileUploadAck struct {
	Origin         string
	Destination    string
	MetaHash       [32]byte
	UploadedChunks []uint64
	Signature      Signature
}

type UploadedFileRequest struct {
	Origin   string
	MetaHash [32]byte
	Nonce    [32]byte
}

type UploadedFileReply struct {
	Origin      string
	Destination string
	OwnedChunks []uint64
	HopLimit    uint32
	MetaHash 	[32]byte
	Signature   Signature
}

// Hash functions for structs
func (b *Block) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write(b.PrevHash[:])
	h.Write(b.Nonce[:])
	_ = binary.Write(h, binary.LittleEndian, uint32(len(b.Transactions)))
	for _, t := range b.Transactions {
		th := t.Hash()
		h.Write(th[:])
	}
	copy(out[:], h.Sum(nil))
	return
}

func (t *TxPublish) Hash() (out [32]byte) {
	h := sha256.New()
	if t.File != nil {
		_ = binary.Write(h, binary.LittleEndian, uint32(len(t.File.Name)))
		h.Write([]byte(t.File.Name))
		h.Write(t.File.MetafileHash)
	}
	if t.Mapping != nil {
		h.Write([]byte(t.Mapping.Identity))
		h.Write(t.Mapping.PublicKey)
		h.Write(t.Mapping.Signature)
	}
	copy(out[:], h.Sum(nil))
	return
}

func (rm *RumorMessage) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write([]byte(rm.Origin))
	_ = binary.Write(h, binary.LittleEndian, rm.ID)
	h.Write([]byte(rm.Text))
	copy(out[:], h.Sum(nil))
	return
}

func (pm *PrivateMessage) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write([]byte(pm.Origin))
	_ = binary.Write(h, binary.LittleEndian, pm.ID)
	h.Write([]byte(pm.Text))
	h.Write([]byte(pm.Destination))
	copy(out[:], h.Sum(nil))
	return
}

func (id *IdentityPKeyMapping) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write([]byte(id.Identity))
	h.Write(id.PublicKey)
	copy(out[:], h.Sum(nil))
	return
}

// The nonce from UploadedFileRequest should be given
func (ufr *UploadedFileReply) Hash(nonce [32]byte) (out [32]byte) {
	h := sha256.New()
	h.Write([]byte(ufr.Origin))
	h.Write([]byte(ufr.Destination))
	for _, chunk := range ufr.OwnedChunks {
		_ = binary.Write(h, binary.LittleEndian, chunk)
	}
	h.Write(ufr.MetaHash[:])
	h.Write(nonce[:])
	copy(out[:], h.Sum(nil))
	return
}

// The nonce from FileUploadMessage should be given. The array of chunks is the
// chunks selected in UploadedChunks
func (fua *FileUploadAck) Hash(chunks [][]byte, nonce [32]byte) (out [32]byte) {
	if len(chunks) != len(fua.UploadedChunks) {
		return
	}
	h := sha256.New()
	h.Write([]byte(fua.Origin))
	h.Write([]byte(fua.Destination))
	for index, chunk := range chunks {
		_ = binary.Write(h, binary.LittleEndian, fua.UploadedChunks[index])
		h.Write(chunk)
	}
	h.Write(nonce[:])
	copy(out[:], h.Sum(nil))
	return
}

// Sends a GossipPacket at a specific host. A connection can be specified or can be nil.
// If it is nil, a new connection is openend on a random port. If a connection is given,
// it has to be unconnected. Thus, if the connection is opened using Dial, SendMessage
// will throw an error
func SendMessage(address string, packet interface{}, conn *net.UDPConn) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return errors.New("Cannot resolve peer address: " + err.Error())
	}

	if conn == nil {
		conn, err = net.ListenUDP("udp", nil)
		if err != nil {
			return errors.New("Cannot open new connection: " + err.Error())
		}
	}

	packetByte, err := protobuf.Encode(packet)
	if err != nil {
		return errors.New("Cannot encode packet: " + err.Error())
	}

	_, err = conn.WriteToUDP(packetByte, udpAddr)
	if err != nil {
		return errors.New("Cannot send message: " + err.Error())
	}

	return nil
}

// Broadcast a message to a list of hosts. This uses SendMessage, so the connection can be nil
// (please refer to the function SendMessage for more information). Also, an optional sender
// can be specified. If it's the case, the message will be broadcast to every hosts except the sender
func BroadcastMessage(hosts []string, message interface{}, sender *string, conn *net.UDPConn) []error {
	errorList := make([]error, 0)
	for _, host := range hosts {
		if sender != nil && host == *sender {
			continue
		}
		err := SendMessage(host, message, conn)
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	return errorList
}
