package syncbox

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
)

// constants for protocol
const (
	RequestPrefix  = 'q'
	ResponsePrefix = 's'

	PacketDataSize  = 1024
	PacketAddrSize  = 8 // roughly allow 4000 GB message size
	PacketTotalSize = 1040

	ByteDelim   = byte(4)
	StringDelim = string(ByteDelim)

	TypeIdentity    = "IDENTITY"
	TypeDigest      = "DIGEST"
	TypeSyncRequest = "SYNC-REQUEST"
	TypeFile        = "FILE"

	StatusOK  = 200
	StatusBad = 400

	MessageAccept = "ACCEPT"
	MessageDeny   = "DENY"

	SyncboxServerUsername = "SYNCBOX-SERVER"
	SyncboxServerPwd      = "SYNCBOX-SERVER-PWD"
	SyncboxServerDevice   = "SYNCBOX-SERVER-DEVICE"
)

// Packet is a fixed length message as the basic element to send acrosss network
type Packet struct {
	Size     [PacketAddrSize]byte // size is the maximun number of Sequence for packets consist of this message
	Sequence [PacketAddrSize]byte
	Data     [PacketDataSize]byte
}

func (packet *Packet) String() string {
	return ToString(packet)
}

// NewPacket instantiates a packet
func NewPacket(size int64, sequence int64, data [PacketDataSize]byte) (*Packet, error) {
	packet := &Packet{
		Data: data,
	}
	if err := packet.SetSize(size); err != nil {
		return nil, err
	}
	if err := packet.SetSequence(sequence); err != nil {
		return nil, err
	}
	return packet, nil
}

func intToBinary(num int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, num)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func binaryToInt(bin []byte) (int64, error) {
	var num int64
	buf := bytes.NewReader(bin)
	err := binary.Read(buf, binary.LittleEndian, &num)
	if err != nil {
		return 0, err
	}
	return num, nil
}

// SetSize sets the total message size to the packet,
// maximum size is 2 ^ (PacketAddrSize * 8)
func (packet *Packet) SetSize(size int64) error {
	bytes, err := intToBinary(size)
	if err != nil {
		return err
	}
	if len(bytes) > PacketAddrSize {
		return ErrorExceedsAddrLength
	}
	copy(packet.Size[:], bytes)
	return nil
}

// GetSize gets the size of the packet
func (packet *Packet) GetSize() (int64, error) {
	num, err := binaryToInt(packet.Size[:])
	if err != nil {
		return 0, err
	}
	return num, nil
}

// SetSequence sets the sequence of the packet
func (packet *Packet) SetSequence(sequence int64) error {
	bytes, err := intToBinary(sequence)
	if err != nil {
		return err
	}
	if len(bytes) > PacketAddrSize {
		return ErrorExceedsAddrLength
	}
	copy(packet.Sequence[:], bytes)
	return nil
}

// GetSequence get sequence of the packet
func (packet *Packet) GetSequence() (int64, error) {
	num, err := binaryToInt(packet.Sequence[:])
	if err != nil {
		return 0, err
	}
	return num, nil
}

// ToBytes transfer a Packet to fixed length byte array
func (packet *Packet) ToBytes() [PacketTotalSize]byte {
	var bytes [PacketTotalSize]byte
	copy(bytes[0:PacketAddrSize], packet.Size[:])
	copy(bytes[PacketAddrSize:2*PacketAddrSize], packet.Sequence[:])
	copy(bytes[2*PacketAddrSize:PacketTotalSize], packet.Data[:])
	return bytes
}

// RebornPacket reborn a packet from a fixed length byte array
func RebornPacket(data [PacketTotalSize]byte) *Packet {
	var packet Packet
	copy(packet.Size[:], data[0:PacketAddrSize])
	copy(packet.Sequence[:], data[PacketAddrSize:2*PacketAddrSize])
	copy(packet.Data[:], data[2*PacketAddrSize:PacketTotalSize])
	return &packet
}

// Serialize transfer some data (a request/response) to series of packets
func Serialize(data []byte) ([]*Packet, error) {
	size := int64(math.Ceil(float64(len(data)) / PacketDataSize))
	var packets []*Packet
	var sequence int64
	for sequence = 0; sequence < size; sequence++ {
		var payload [PacketDataSize]byte
		if sequence == size-1 {
			copy(payload[:], data[sequence*PacketDataSize:len(data)])
		} else {
			copy(payload[:], data[sequence*PacketDataSize:(sequence+1)*PacketDataSize])
		}
		packet, err := NewPacket(size, sequence, payload)
		if err != nil {
			return nil, err
		}
		packets = append(packets, packet)
	}
	return packets, nil
}

// Deserialize transfer a series of packets to some data (a request or response)
func Deserialize(packets []*Packet) []byte {
	var packetsCount = int64(len(packets))
	var dataSize = packetsCount * PacketDataSize
	data := make([]byte, dataSize)
	var offset int64
	for _, packet := range packets {
		copy(data[offset:offset+PacketDataSize], packet.Data[:])
		offset += PacketDataSize
	}
	return data
}

// Request structure for request
type Request struct {
	Username string
	Password string
	Device   string
	DataType string
	Data     []byte
}

func (req *Request) String() string {
	return ToString(req)
}

// Response structure for response
type Response struct {
	Status  int
	Message string
	Data    []byte
}

func (res *Response) String() string {
	return ToString(res)
}

// IdentityRequest is the Request data type of user identity
type IdentityRequest struct {
	Username string
}

func (req *IdentityRequest) String() string {
	return ToString(req)
}

// DigestRequest is the Request data type of a file tree digest
type DigestRequest struct {
	Dir *Dir
}

func (req *DigestRequest) String() string {
	return ToString(req)
}

// SyncRequest is the Request data type of a file CRUD request
type SyncRequest struct {
	Action     string
	File       *File
	UnrootPath string
}

func (req *SyncRequest) String() string {
	return ToString(req)
}

// FileRequest is the Request data type of CRUD on file content
type FileRequest struct {
	File       *File
	UnrootPath string
	Content    []byte
}

func (req *FileRequest) String() string {
	return ToString(req)
}

// ToJSON converts request to JSON string
func (req *Request) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// RebornRequest reborn request from JSON string
func RebornRequest(jsonStr string) (*Request, error) {
	jsonBytes := []byte(jsonStr)
	restoredReq := Request{}
	if err := json.Unmarshal(jsonBytes, &restoredReq); err != nil {
		return nil, err
	}
	return &restoredReq, nil
}

// ToJSON converts response to JSON string
func (res *Response) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// RebornResponse reborn response from JSON string
func RebornResponse(jsonStr string) (*Response, error) {
	jsonBytes := []byte(jsonStr)
	restoredRes := Response{}
	if err := json.Unmarshal(jsonBytes, &restoredRes); err != nil {
		return nil, err
	}
	return &restoredRes, nil
}
