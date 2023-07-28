package main

import (
	"github.com/lvkeliang/P2Pin2/torrent"
	"log"
)

func main() {
	//inPath := "F:\\torrent\\[ANi] 殭屍 100～在成為殭屍前要做的 100 件事～ - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT].mp4.torrent"
	outPath := "F:\\Gostudy\\P2Pin2"

	newtorrent, err := torrent.NewTorrentFile("F:\\pictures\\OIG.jpg", "http://localhost:8090/announce", 12*1024)
	if err != nil {
		log.Fatal(err)
	}
	err = newtorrent.SaveTorrentFile("./have/result.json")

	t, err := torrent.LoadTorrentFile("./have/result.json")
	if err != nil {
		log.Fatal(err)
	}

	//tf, err := torrent.Open(inPath)
	//if err != nil {
	//	log.Fatal(err)
	//}

	err = t.DownloadToFile(outPath)
	if err != nil {
		log.Fatal(err)
	}

}
