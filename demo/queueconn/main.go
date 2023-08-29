package main

import (
	"bufio"
	"bytes"
	"fmt"
	webrtcLib "github.com/ninjahome/webrtc/mobile"
	"os"
	"strconv"
	"time"
)

func main() {
	var file, err = os.Open("1.txt")
	if err != nil {
		panic(err)
	}
	var reader = bufio.NewReader(file)
	var datas []*webrtcLib.DataNode
	var idx uint8 = 0
	for {
		var lineData, _, errR = reader.ReadLine()
		if errR != nil {
			fmt.Println(errR)
			break
		}
		if len(lineData) == 0 || lineData[0] != '*' {
			continue
		}
		var start = bytes.Index(lineData, []byte("******>>>qc read node: {seq:"))
		var end = bytes.Index(lineData, []byte("\tisKey:"))
		start = start + len("******>>>qc read node: {seq:")
		var seqStr = string(lineData[start:end])

		start = bytes.Index(lineData, []byte("isKey:")) + len("isKey:")
		end = bytes.Index(lineData, []byte("\tbuf:"))
		var keyStr = string(lineData[start:end])

		var seq, errA = strconv.Atoi(seqStr)
		if errA != nil {
			panic(errA)
		}
		idx = uint8(seq % 256)
		var node = &webrtcLib.DataNode{
			Seq:   uint32(seq),
			IsKey: keyStr == "true",
			Buf:   []byte{idx},
		}
		datas = append(datas, node)
	}

	var sq = &webrtcLib.SortedQueue{
		Pointer: webrtcLib.QCNullPointer,
		Pool:    make([]*webrtcLib.DataNode, webrtcLib.QCNodePool),
	}
	var sig = make(chan struct{}, webrtcLib.QCNodePool)
	go func() {
		for _, node := range datas {
			_ = sq.Product(node)
			time.Sleep(time.Millisecond * 200)
			sig <- struct{}{}
		}
	}()
	var seqCh = make(chan uint32, 1024)
	go func() {
		select {
		case seq := <-seqCh:
			fmt.Println(seq)
		}
	}()
	for {
		select {
		case <-sig:
			var buf = sq.Consume(seqCh)
			if buf == nil {
				continue
			}
			fmt.Println(buf)
		}
	}
}
