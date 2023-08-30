package webrtcLib

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
)

const (
	H264TypMask       = 0x1f
	MaxConnBufferSize = 1 << 22
	MaxInBufferSize   = 1 << 10
	VideoAvcLen       = 4
)

var (
	VideoAvcStart = []byte{0x00, 0x00, 0x00, 0x01}
	_inst         = &AppInst{}
)

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

type CallBack interface {
	NewVideoData(typ int, h264data []byte)
	AnswerCreated(string)
	OfferCreated(string)
}

type AppInst struct {
	appLocker sync.RWMutex

	callback CallBack
	p2pConn  NinjaConn

	localVideoPacket chan []byte
	localAudioPacket chan []byte
}

func initSdk(cb CallBack) {
	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()

	_inst.localVideoPacket = make(chan []byte, MaxInBufferSize)
	_inst.localAudioPacket = make(chan []byte, MaxInBufferSize)
	_inst.callback = cb
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func (ai *AppInst) RawCameraData() ([]byte, error) {
	var pkt, ok = <-ai.localVideoPacket
	if !ok {
		return nil, io.EOF
	}
	//fmt.Println("=====>>> app got camera data:", len(pkt))
	return pkt, nil
}
func (ai *AppInst) RawMicroData() ([]byte, error) {
	var pkt, ok = <-ai.localAudioPacket
	if !ok {
		return nil, io.EOF
	}
	return pkt, nil
}

func (ai *AppInst) EndCall(err error) {
	fmt.Println("======>>>the call will be end:", err)
}

func (ai *AppInst) AnswerForCallerCreated(answer string) {
	ai.callback.AnswerCreated(answer)
}
func (ai *AppInst) OfferForCalleeCreated(offer string) {
	ai.callback.OfferCreated(offer)
}

func (ai *AppInst) GotVideoData(p []byte) (n int, err error) {
	return h254Write(p, ai.callback.NewVideoData)
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func h254Write(p []byte, callback func(typ int, h264data []byte)) (n int, err error) {
	if len(p) < 5 {
		fmt.Println("======>>>invalid rtp packets:", p)
		return 0, nil
	}

	var startIdx = bytes.Index(p, VideoAvcStart)
	if startIdx != 0 {
		return 0, fmt.Errorf("invalid h64 stream data\n%v", hex.EncodeToString(p))
	}

	var typ = int(p[VideoAvcLen] & H264TypMask)
	var origLen = len(p)
	p = p[VideoAvcLen:]
	if typ == 7 {
		startIdx = bytes.Index(p, VideoAvcStart)
		if startIdx < 0 {
			callback(typ, p)
			return origLen, nil
		}
		callback(typ, p[:startIdx])

		p = p[startIdx+VideoAvcLen:]
		var nextTyp = int(p[0] & H264TypMask)
		if nextTyp != 8 {
			return 0, fmt.Errorf("error pps frame")
		}
		callback(nextTyp, p)
		return origLen, nil
	}

	if typ > 0 {
		callback(typ, p)
		return origLen, nil
	}

	return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
}
