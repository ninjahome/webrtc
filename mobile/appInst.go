package webrtcLib

import (
	"bytes"
	"fmt"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"io"
	"sync"
)

const (
	MaxBufferSize = 1 << 10
)

type AppInst struct {
	appLocker sync.RWMutex
	CallBack
	//queue        deque.Deque[[]byte]
	videoRawBuff     chan []byte //deque.Deque[[]byte]
	p2pConn          *webrtc.PeerConnection
	builder          *samplebuilder.SampleBuilder
	x264Writer       *h264writer.H264Writer
	localVideoPacket chan []byte
}

func (ai *AppInst) GotRtp(packet *rtp.Packet) error {
	return ai.x264Writer.WriteRTP(packet)
}

func (ai *AppInst) StatusChanged(b bool) {
	ai.P2pConnected()
}

func (ai *AppInst) RawCameraData() ([]byte, error) {
	var pkt, ok = <-ai.localVideoPacket
	if !ok {
		return nil, io.EOF
	}
	return pkt, nil
}

var (
	startCode = []byte{0x00, 0x00, 0x00, 0x01}
	sCodeLen  = len(startCode)
)

const (
	H264TypMask = 0x1f
)

func h254Write(p []byte, callback func(typ int, h264data []byte)) (n int, err error) {
	if len(p) < 5 {
		fmt.Println("======>>>invalid rtp packets:", p)
		return 0, nil
	}

	var startIdx = bytes.Index(p, startCode)
	if startIdx != 0 {
		return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
	}

	var typ = int(p[sCodeLen] & H264TypMask)
	var origLen = len(p)
	p = p[sCodeLen:]
	if typ == 7 {
		startIdx = bytes.Index(p, startCode)
		if startIdx < 0 {
			return 0, fmt.Errorf("error sps frame")
		}
		callback(typ, p[:startIdx])

		p = p[startIdx+sCodeLen:]
		var nextTyp = int(p[0] & H264TypMask)
		if nextTyp != 8 {
			return 0, fmt.Errorf("error pps frame")
		}
		callback(nextTyp, p)
		return origLen, nil
	}

	if typ > 0 {
		callback(typ, p)
		if typ != 1 && typ != 5 {
			fmt.Println("==================>new type", typ)
		}
		return origLen, nil
	}

	return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
}

func (ai *AppInst) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}
	//fmt.Println("======>>>sample data:", hex.EncodeToString(p))
	var rawData = make([]byte, len(p))
	copy(rawData, p)
	//ai.videoRawBuff = append(ai.videoRawBuff, p...)
	return h254Write(rawData, ai.NewVideoData)
	//return len(p), nil
}

var _inst = &AppInst{}

type CallBack interface {
	NewVideoData(typ int, h264data []byte)
	P2pConnected()
}

func (ai *AppInst) readingFromPeer() {
	defer fmt.Println("======>>> reading go thread exit")
	fmt.Println("======>>> start to read data from peer")
}
