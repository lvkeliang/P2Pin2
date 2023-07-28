package protocol

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"github.com/lvkeliang/P2Pin2/application"
	"github.com/lvkeliang/P2Pin2/logic"
	"log"
	"runtime"
	"time"
)

// MaxBlockSize is the largest number of bytes a request can ask for
const MaxBlockSize = 16384

// MaxBacklog is the number of unfulfilled requests a client can have in its pipeline
const MaxBacklog = 5

// Torrent holds data required to download a torrent from a list of peers
type Torrent struct {
	Peers       []logic.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type PieceProgress struct {
	index      int
	client     *application.Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

func (state *PieceProgress) readMessage() error {
	msg, err := state.client.Read() // this call blocks
	if err != nil {
		return err
	}

	if msg == nil { // keep-alive
		return nil
	}

	switch msg.ID {
	case logic.MsgUnchoke:
		state.client.Choked = false
	case logic.MsgChoke:
		state.client.Choked = true
	case logic.MsgHave:
		index, err := logic.ParseHave(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
	case logic.MsgPiece:
		n, err := logic.ParsePiece(state.index, state.buf, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}
	return nil
}

func attemptDownloadPiece(c *application.Client, pw *pieceWork) ([]byte, error) {
	state := PieceProgress{ // 创建一个 PieceProgress 类型的变量，用于存储下载进度
		index:  pw.index,                // 索引
		client: c,                       // 客户端
		buf:    make([]byte, pw.length), // 缓冲区
	}

	// 设置一个 60 秒的截止时间，以帮助解决无响应的对等点问题
	c.Conn.SetDeadline(time.Now().Add(60 * time.Second))
	defer c.Conn.SetDeadline(time.Time{}) // 禁用截止时间
	for state.downloaded < pw.length {    // 当下载的字节数小于文件块的长度时，继续循环
		// 如果客户端未被阻塞，则发送请求直到达到最大积压量或请求完成
		if !state.client.Choked {
			for state.backlog < MaxBacklog && state.requested < pw.length {
				blockSize := MaxBlockSize // 设置块大小为最大块大小
				// 如果剩余的字节数小于块大小，则将块大小设置为剩余的字节数
				if pw.length-state.requested < blockSize {
					blockSize = pw.length - state.requested
				}

				//err := c.SendRequest(pw.index, state.requested, blockSize) // 发送请求
				req := logic.FormatRequest(pw.index, state.requested, blockSize)
				fmt.Printf("pw.index: %v, state.requested: %v, blockSize: %v\n", pw.index, state.requested, blockSize)
				_, err := c.Conn.Write(req.Serialize())

				if err != nil { // 如果发送请求出错，则返回空字节数组和错误

					return nil, err
				}

				state.backlog++              // 增加积压量
				state.requested += blockSize // 增加已请求的字节数

			}
		}

		err := state.readMessage() // 读取消息
		if err != nil {            // 如果读取消息出错，则返回空字节数组和错误
			return nil, err
		}
	}

	return state.buf, nil // 当下载完成时，返回缓冲区中的数据和空错误

}

func checkIntegrity(pw *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("Index %d failed integrity check", pw.index)
	}
	return nil
}

func (t *Torrent) startDownloadWorker(peer logic.Peer, workQueue chan *pieceWork, results chan *pieceResult) {
	c, err := application.New(peer, t.PeerID, t.InfoHash)
	if err != nil {
		log.Printf("Could not handshake with %s. Disconnecting\n", peer.IP)
		return
	}

	defer c.Conn.Close()
	log.Printf("Completed handshake with %s\n", peer.IP)
	c.SendUnchoke()
	c.SendInterested()
	for pw := range workQueue {
		if !c.Bitfield.HasPiece(pw.index) {
			workQueue <- pw // Put piece back on the queue
			continue
		}

		// Download the piece
		buf, err := attemptDownloadPiece(c, pw)

		if err != nil {
			log.Println("Exiting", err)
			workQueue <- pw // Put piece back on the queue
			return
		}

		err = checkIntegrity(pw, buf)
		if err != nil {
			log.Printf("Piece #%d failed integrity check\n", pw.index)
			workQueue <- pw // Put piece back on the queue
			continue
		}

		c.SendHave(pw.index)
		results <- &pieceResult{pw.index, buf}
	}
}

func (t *Torrent) calculateBoundsForPiece(index int) (begin int, end int) {
	begin = index * t.PieceLength
	end = begin + t.PieceLength
	if end > t.Length {
		end = t.Length
	}
	return begin, end
}

func (t *Torrent) calculatePieceSize(index int) int {
	begin, end := t.calculateBoundsForPiece(index)
	return end - begin
}

// Download downloads the torrent. This stores the entire file in memory.
func (t *Torrent) Download() ([]byte, error) {
	log.Println("Starting download for", t.Name)
	// Init queues for workers to retrieve work and send results
	workQueue := make(chan *pieceWork, len(t.PieceHashes))
	results := make(chan *pieceResult)
	for index, hash := range t.PieceHashes {
		length := t.calculatePieceSize(index)
		workQueue <- &pieceWork{index, hash, length}
	}
	// Start workers
	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, workQueue, results)
	}
	// Collect results into a buffer until full
	buf := make([]byte, t.Length)
	donePieces := 0
	for donePieces < len(t.PieceHashes) {
		res := <-results
		begin, end := t.calculateBoundsForPiece(res.index)
		copy(buf[begin:end], res.buf)
		donePieces++
		percent := float64(donePieces) / float64(len(t.PieceHashes)) * 100
		numWorkers := runtime.NumGoroutine() - 1 // subtract 1 for main thread
		log.Printf("(%0.2f%%) Downloaded piece #%d from %d peers\n", percent, res.index, numWorkers)
	}
	close(workQueue)

	return buf, nil
}
