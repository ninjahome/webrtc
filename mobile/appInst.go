package webrtcLib

import (
	"bytes"
	"fmt"
	"io"
	"sync"
)

const (
	H264TypMask           = 0x1f
	MaxConnBufferSize     = 1 << 22
	MaxDataConnBufferSize = 1 << 16
	MaxInBufferSize       = 1 << 10

	IceUdpMtu = 1 << 10
)

var (
	startCode = []byte{0x00, 0x00, 0x00, 0x01}
	sCodeLen  = len(startCode)
	_inst     = &AppInst{}
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

	CallBack
	p2pConn NinjaConn
	//p2pConn *NinjaRtpConn
	//p2pConn *NinjaDataConn

	localVideoPacket chan []byte
	localAudioPacket chan []byte
}

func initSdk(cb CallBack) {
	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()

	_inst.localVideoPacket = make(chan []byte, MaxInBufferSize)
	_inst.localAudioPacket = make(chan []byte, MaxInBufferSize)
	_inst.CallBack = cb
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

func (ai *AppInst) AnswerForCallerCreated(a string) {
	ai.CallBack.AnswerCreated(a)
}
func (ai *AppInst) OfferForCalleeCreated(o string) {
	ai.CallBack.OfferCreated(o)
}

func (ai *AppInst) GotVideoData(p []byte) (n int, err error) {
	return h254Write(p, ai.NewVideoData)
	//return h254Write2(p, ai.NewVideoData)
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
			callback(typ, p)
			return origLen, nil
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
		return origLen, nil
	}

	return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
}
func h254Write2(p []byte, callback func(typ int, h264data []byte)) (n int, err error) {
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
	callback(typ, p)
	return origLen, nil

	//if typ == 7 {
	//	startIdx = bytes.Index(p, startCode)
	//	if startIdx < 0 {
	//		return 0, fmt.Errorf("error sps frame")
	//	}
	//	callback(typ, p[:startIdx])
	//
	//	p = p[startIdx+sCodeLen:]
	//	var nextTyp = int(p[0] & H264TypMask)
	//	if nextTyp != 8 {
	//		return 0, fmt.Errorf("error pps frame")
	//	}
	//	callback(nextTyp, p)
	//	return origLen, nil
	//}
	//
	//if typ > 0 {
	//	callback(typ, p)
	//	//if typ != 1 && typ != 5 {
	//	//	fmt.Println("==================>new type", typ)
	//	//}
	//	return origLen, nil
	//}
	//return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
}
