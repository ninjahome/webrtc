package webrtcLib

import (
	"bytes"
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
var foundKeyFrame = false

func SendVideoToPeer(data []byte) error {
	if _inst.p2pConn.ConnectionState() != webrtc.PeerConnectionStateConnected {
		return nil
	}
	var rawData = make([]byte, len(data))
	copy(rawData, data)

	if !foundKeyFrame {
		var idx = bytes.Index(rawData, startCode)
		if idx < 0 {
			return nil
		}
		//fmt.Println("======>>>rawData:", rawData[idx+sCodeLen], hex.EncodeToString(rawData))
		if rawData[idx+sCodeLen]&H264TypMask == 7 || rawData[idx+sCodeLen]&H264TypMask == 8 {
			foundKeyFrame = true
			fmt.Println("======>>> found key frame")
		}
	}

	if !foundKeyFrame {
		fmt.Println("======>>>no key frame yet")
		return nil
	}

	_inst.localVideoPacket <- rawData
	return nil
}

func StartVideo(offerStr string, cb CallBack) error {
	if len(offerStr) < 10 || cb == nil {
		return fmt.Errorf("error parametor for start video")
	}

	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()

	_inst.videoRawBuff = make(chan []byte, MaxBufferSize)
	_inst.localVideoPacket = make(chan []byte, MaxBufferSize)
	_inst.CallBack = cb
	_inst.x264Writer = h264writer.NewWith(_inst)
	var peerConnection, err = createP2pConnect(offerStr, _inst)

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
