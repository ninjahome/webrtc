package webrtcLib

import (
	"bytes"
	"fmt"
	"github.com/ninjahome/webrtc/mobile/conn"
	"github.com/ninjahome/webrtc/relay-server"
	"github.com/zaf/g711"
	"io"
	"net/http"
	"time"
)

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func StartCall(hasVideo, isCaller bool, sid string, cb CallBack) error {
	initSdk(cb)
	var typ = relay.STCallerOffer
	if !isCaller {
		typ = relay.STCalleeOffer
	}

	var peerConnection, err = conn.CreateCallerRtpConn(hasVideo, _inst)
	if err != nil {
		return err
	}

	_inst.p2pConn = peerConnection

	var offer, errOffer = peerConnection.GetOffer(typ, "alice-to-bob")
	if errOffer != nil {
		return errOffer
	}
	//fmt.Println(offer)
	_inst.callback.OfferCreated(offer) //TODO:: refactor this method

	return nil
}

func EndCallByController() {
	if _inst.p2pConn == nil {
		return
	}
	_inst.p2pConn.Close()
	_inst.p2pConn = nil
	if _inst.localVideoPacket != nil {
		close(_inst.localVideoPacket)
		_inst.localVideoPacket = nil
	}
	if _inst.localAudioPacket != nil {
		close(_inst.localAudioPacket)
		_inst.localAudioPacket = nil
	}
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
	if _inst.p2pConn == nil {
		fmt.Println("======>>>[SetAnswerForOffer] connection closed")
		return
	}
	var err = _inst.p2pConn.SetRemoteDesc(answer)
	if err != nil {
		fmt.Println("======>>>SetAnswerForOffer err:", err)
		_inst.EndCallByInnerErr(err)
	}
}

var relayClient = &http.Client{
	Timeout: time.Second * 15,
	Transport: &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	},
}

func SdpToRelay(url, sdp string) string {
	var reader = bytes.NewBuffer([]byte(sdp))
	var response, err = relayClient.Post(url, "application/json", reader)
	if err != nil {
		fmt.Println("======>>> post to relay server err:", err)
		return ""
	}
	defer response.Body.Close()
	var body, errRes = io.ReadAll(response.Body)
	if errRes != nil {
		fmt.Println("======>>> read response body err:", err)
		return ""
	}
	return string(body)
}
