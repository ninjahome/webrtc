package webrtcLib

import (
	"fmt"
	"github.com/ninjahome/webrtc/mobile/conn"
	"io"
)

const ()

var (
	_inst = &AppInst{}
)

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

type CallBack interface {
	NewVideoData(typ int, h264data []byte)
	NewAudioData(data []byte)
	AnswerCreated(string)
	OfferCreated(string)
	Connected()
	Disconnected()
}

type AppInst struct {
	callback CallBack
	p2pConn  conn.NinjaConn

	localVideoPacket chan []byte
	localAudioPacket chan []byte
}

func initSdk(cb CallBack) {
	_inst.localVideoPacket = make(chan []byte, conn.MaxInBufferSize)
	_inst.localAudioPacket = make(chan []byte, conn.MaxInBufferSize)
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

func (ai *AppInst) EndCallByInnerErr(err error) {
	fmt.Println("======>>>the call will be end:", err)
	ai.callback.Disconnected()

	if _inst.localVideoPacket != nil {
		close(_inst.localVideoPacket)
		_inst.localVideoPacket = nil
	}
	if _inst.localAudioPacket != nil {
		close(_inst.localAudioPacket)
		_inst.localAudioPacket = nil
	}
}

func (ai *AppInst) AnswerForCallerCreated(answer string) {
	ai.callback.AnswerCreated(answer)
}

func (ai *AppInst) GotVideoData(p []byte) (n int, err error) {
	return conn.H254Write(p, ai.callback.NewVideoData)
}

func (ai *AppInst) GotAudioData(p []byte) (n int, err error) {
	ai.callback.NewAudioData(p)
	return len(p), nil
}

func (ai *AppInst) CallStart() {
	ai.callback.Connected()
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/
