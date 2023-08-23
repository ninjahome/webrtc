package webrtcLib

import (
	"fmt"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
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
	var rawData = make([]byte, len(data))
	copy(rawData, data)
	//fmt.Printf("\n%s\n", hex.EncodeToString(rawData))
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
	_inst.x264Writer = h264writer.NewWith(_inst)
	var peerConnection, err = createP2pConnect(offerStr, _inst.build)

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
