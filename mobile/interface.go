package webrtcLib

import (
	"bytes"
	"fmt"
	"github.com/ninjahome/webrtc/mobile/conn"
	"github.com/ninjahome/webrtc/relay-server"
	"github.com/zaf/g711"
	"time"
)

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func StartVideo(isCaller bool, cb CallBack) error {
	initSdk(cb)
	// CreateCallerIceConn(CallTypeVideo, _inst) //CreateCallerDataConn(_inst) //CreateCallerRtpConn(_inst)
	var typ = relay.STCallerOffer
	if !isCaller {
		typ = relay.STCalleeOffer
	}
	var peerConnection, err = conn.CreateCallerRtpConn(typ, _inst)
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
	// CreateCalleeIceConn(CallTypeVideo, offerStr, _inst) //CreateCalleeDataConn(offerStr, _inst) //CreateCalleeRtpConn(offerStr, _inst)

	var peerConnection, err = conn.CreateCalleeRtpConn(offerStr, _inst)
	if err != nil {
		return err
	}
	_inst.p2pConn = peerConnection

	return nil
}

func EndCall() {
	_inst.appLocker.Lock()
	defer _inst.appLocker.Unlock()
	close(_inst.localVideoPacket)
	close(_inst.localAudioPacket)
	_inst.p2pConn.Close()
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/
var foundKeyFrame = false //TODO:: refactor this variable

func SendVideoToPeer(data []byte) error {
	if _inst.p2pConn == nil || _inst.p2pConn.IsConnected() == false {
		return nil
	}
	var rawData = make([]byte, len(data))
	copy(rawData, data)

	if !foundKeyFrame {
		var idx = bytes.Index(rawData, conn.VideoAvcStart)
		if idx < 0 {
			return nil
		}
		//fmt.Println("======>>>rawData:", rawData[idx+sCodeLen], hex.EncodeToString(rawData))
		if rawData[idx+conn.VideoAvcLen]&conn.H264TypMask == 7 ||
			rawData[idx+conn.VideoAvcLen]&conn.H264TypMask == 8 {
			foundKeyFrame = true
			fmt.Println("======>>> found key frame")
		}
	}

	if !foundKeyFrame {
		fmt.Println("======>>>no key frame yet")
		return nil
	}
	//fmt.Println("======>>>send camera data from app:", len(rawData))
	_inst.localVideoPacket <- rawData
	return nil
}

func SendAudioToPeer(data []byte) error {
	if _inst.p2pConn == nil || _inst.p2pConn.IsConnected() == false {
		return nil
	}
	var rawData = make([]byte, len(data))
	copy(rawData, data)

	var pcmuData = g711.EncodeUlaw(rawData)

	_inst.localAudioPacket <- pcmuData
	return nil
}

func SetAnswerForOffer(answer string) {
	var err = _inst.p2pConn.SetRemoteDesc(answer)
	if err != nil {
		fmt.Println("======>>>SetAnswerForOffer err:", err)
		_inst.EndCall(err)
	}
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func TestFileData(cb CallBack, data []byte) {

	var startIdx = bytes.Index(data, conn.VideoAvcStart)
	if startIdx != 0 {
		fmt.Println("======>>> invalid h264 stream")
		return
	}
	sleepTime := time.Millisecond * time.Duration(33)
	data = data[conn.VideoAvcLen:]
	for {
		var typ = int(data[0] & conn.H264TypMask)
		if typ == 7 || typ == 8 {
			startIdx = bytes.Index(data, conn.VideoAvcStart)
			if startIdx < 0 {
				fmt.Println("======>>> find sps or pps err")
				return
			}
			var spsOrPssData = data[0:startIdx]
			cb.NewVideoData(typ, spsOrPssData)
			data = data[startIdx+conn.VideoAvcLen:]
			continue

		}
		if typ > 0 {
			startIdx = bytes.Index(data, conn.VideoAvcStart)
			if startIdx < 0 {
				fmt.Println("======>>> found last frame")
				cb.NewVideoData(typ, data)
				return
			}
			var vdata = data[0:startIdx]
			cb.NewVideoData(typ, vdata)
			time.Sleep(sleepTime)

			data = data[startIdx+conn.VideoAvcLen:]
			continue
		}

	}
}

func AudioEncodePcmu(lpcm []byte) []byte {
	return g711.EncodeUlaw(lpcm)
}

func AudioDecodePcmu(pcmu []byte) []byte {
	return g711.DecodeUlaw(pcmu)
}
