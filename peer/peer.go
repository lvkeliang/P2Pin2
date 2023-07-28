package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/lvkeliang/P2Pin2/handshake"
	"github.com/lvkeliang/P2Pin2/logic"
	"github.com/lvkeliang/P2Pin2/torrent"
	"io"
	"log"
	"net"
	"os"
	"time"
)

type TorrentFile struct {
	InfoHash    [20]byte   `json:"info_hash"`
	PieceHashes [][20]byte `json:"piece_hashes"`
}

func getBitfield(conn net.Conn, pieceLength int, file os.File) (bitfield []byte, err error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	totalPieces := int(fileInfo.Size()) / pieceLength
	if int(fileInfo.Size())%pieceLength != 0 {
		totalPieces++
	}

	bitfield = make([]byte, totalPieces/8+1)
	for i := 0; i < totalPieces; i++ {
		bitfield[i/8] |= 1 << (7 - uint(i%8))
	}

	msg := make([]byte, len(bitfield)+5)
	msg[0] = byte(len(bitfield) + 1)
	msg[4] = byte(logic.MsgBitfield)
	copy(msg[5:], bitfield)

	_, err = conn.Write(msg)
	return bitfield, err
}

func Handshake(conn net.Conn, infohash, peerID [20]byte) (*handshake.Handshake, error) {
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Disable the deadline

	req := handshake.New(infohash, peerID)
	_, err := conn.Write(req.Serialize())
	if err != nil {
		return nil, err
	}

	res, err := handshake.Read(conn)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(res.InfoHash[:], infohash[:]) {
		return nil, fmt.Errorf("Expected infohash %x but got %x", res.InfoHash, infohash)
	}
	return res, nil
}

// IntToBytesBigEndian int 转大端 []byte
// 此函数摘自go整型和字节数组之间的转换（https://blog.csdn.net/xuemengrui12/article/details/106056220）
func IntToBytesBigEndian(n int64, bytesLength byte) ([]byte, error) {
	switch bytesLength {
	case 1:
		tmp := int8(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes(), nil
	case 2:
		tmp := int16(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes(), nil
	case 3:
		tmp := int32(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes()[1:], nil
	case 4:
		tmp := int32(n)
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes(), nil
	case 5:
		tmp := n
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes()[3:], nil
	case 6:
		tmp := n
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes()[2:], nil
	case 7:
		tmp := n
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes()[1:], nil
	case 8:
		tmp := n
		bytesBuffer := bytes.NewBuffer([]byte{})
		binary.Write(bytesBuffer, binary.BigEndian, &tmp)
		return bytesBuffer.Bytes(), nil
	}
	return nil, fmt.Errorf("IntToBytesBigEndian b param is invaild")
}

func handleConnection(conn net.Conn, infoHash [20]byte, pieceLength int, bitfie [][20]byte) {

	defer conn.Close()

	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		return
	}

	// 处理握手
	_, err = Handshake(conn, infoHash, peerID)
	if err != nil {
		conn.Close()
		return
	}

	var bit []byte
	for n, _ := range bit {
		bit = append(bit, byte(n))
	}

	var bitfield []byte
	for _, b := range bitfie {
		bitfield = append(bitfield, b[:]...)

	}

	msg := make([]byte, len(bitfield)+5)
	byteLen, err := IntToBytesBigEndian(int64(len(bitfield)+1), 4)
	copy(msg[:4], byteLen)
	msg[4] = byte(logic.MsgBitfield)
	copy(msg[5:], bitfield)

	_, err = conn.Write(msg)

	requests := make(chan struct {
		filename string
		index    int
	})

	go func() {
		for req := range requests {
			filename := req.filename
			index := req.index

			file, err := os.Open(filename)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			buf := make([]byte, pieceLength)
			n, err := file.ReadAt(buf, int64(index*pieceLength))
			if err != nil && err != io.EOF {
				log.Fatal(err)
			}
			_, err = conn.Write(buf[:n])
			if err != nil {
				log.Fatal(err)
			}
		}
	}()

	// 处理请求...
}

func main() {
	t, err := torrent.LoadTorrentFile("./have/result.json")
	infoHash := t.InfoHash
	bitfie := t.PieceHashes
	pieceLength := 12 * 1024

	listener, err := net.Listen("tcp", "localhost:8096")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go handleConnection(conn, infoHash, pieceLength, bitfie)
	}
}
