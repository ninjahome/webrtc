package webrtcLib

import (
	"fmt"
	"github.com/pion/webrtc/v3"
)

var (
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

func SendVideoToPeer(data []byte) error {
	//var rawData = make([]byte, len(data))
	//copy(rawData, data)
	return nil
}

func StartVideo(offerStr string, cb CallBack) error {
	if len(offerStr) < 10 || cb == nil {
		return fmt.Errorf("error parametor for start video")
	}

	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()

	_inst.videoRawBuff = make(chan []byte, MaxBufferSize)
	_inst.CallBack = cb

	var peerConnection, err = createP2pConnect(offerStr, func(bytes []byte) error {
		//fmt.Println("======>>>got peer data:", len(bytes))
		_inst.videoRawBuff <- bytes
		return nil
	})

	if err != nil {
		return err
	}
	_inst.p2pConn = peerConnection

	go _inst.readingFromPeer()

	return nil
}

func StopVideo() {
	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()
	close(_inst.videoRawBuff)
	_ = _inst.p2pConn.Close()
}
