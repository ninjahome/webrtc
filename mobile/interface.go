package webrtcLib

import (
	"bytes"
	"fmt"
	"github.com/pion/webrtc/v3"
	"time"
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
	if _inst.p2pConn.IsConnected() {
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

func StartVideo(cb CallBack) error {
	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()
	initSdk(cb)

	var peerConnection, err = CreateConnectionAsCaller(_inst)
	if err != nil {
		return err
	}
	_inst.p2pConn = peerConnection

	return nil
}

func AnswerVideo(offerStr string, cb CallBack) error {
	if len(offerStr) < 10 || cb == nil {
		return fmt.Errorf("error parametor for start video")
	}

	initSdk(cb)

	var peerConnection, err = CreateConnectAsCallee(offerStr, _inst)
	if err != nil {
		return err
	}
	_inst.p2pConn = peerConnection

	return nil
}

func StopVideo() {
	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()
	close(_inst.localVideoPacket)
	close(_inst.localAudioPacket)
	_inst.p2pConn.Close()
}

func SetAnswerForOffer(answer string) {
	var err = _inst.p2pConn.setRemoteDescription(answer)
	if err != nil {
		_inst.EndCall()
	}
}

func TestFileData(cb CallBack, data []byte) {

	var startIdx = bytes.Index(data, startCode)
	if startIdx != 0 {
		fmt.Println("======>>> invalid h264 stream")
		return
	}
	sleepTime := time.Millisecond * time.Duration(33)
	data = data[sCodeLen:]
	for {
		var typ = int(data[0] & H264TypMask)
		if typ == 7 || typ == 8 {
			startIdx = bytes.Index(data, startCode)
			if startIdx < 0 {
				fmt.Println("======>>> find sps or pps err")
				return
			}
			var spsOrPssData = data[0:startIdx]
			cb.NewVideoData(typ, spsOrPssData)
			data = data[startIdx+sCodeLen:]
			continue

		}
		if typ > 0 {
			startIdx = bytes.Index(data, startCode)
			if startIdx < 0 {
				fmt.Println("======>>> found last frame")
				cb.NewVideoData(typ, data)
				return
			}
			var vdata = data[0:startIdx]
			cb.NewVideoData(typ, vdata)
			time.Sleep(sleepTime)

			data = data[startIdx+sCodeLen:]
			continue
		}

	}
}
