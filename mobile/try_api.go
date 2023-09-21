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
*for test purpose
*
************************************************************************************************************/

func StartVideo(isCaller bool, cb CallBack) error {
	initSdk(cb)
	var typ = relay.STCallerOffer
	if !isCaller {
		typ = relay.STCalleeOffer
	}
	var peerConnection, err = conn.CreateCallerRtpConn(true, _inst)
	if err != nil {
		return err
	}
	_inst.p2pConn = peerConnection

	var offer, errOffer = peerConnection.GetOffer(typ, "alice-to-bob")
	if errOffer != nil {
		return errOffer
	}
	//fmt.Println(offer)
	_inst.callback.OfferCreated(offer)
	return nil
}

func AnswerVideo(offerStr string, cb CallBack) error {
	if len(offerStr) < 10 || cb == nil {
		return fmt.Errorf("error parametor for start video")
	}

	initSdk(cb)

	var peerConnection, err = conn.CreateCalleeRtpConn(true, offerStr, _inst)
	if err != nil {
		return err
	}
	_inst.p2pConn = peerConnection

	return nil
}

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
	var encoded = g711.EncodeUlaw(lpcm)
	return encoded
}

func AudioDecodePcmu(pcmu []byte) []byte {
	var decoded = g711.DecodeUlaw(pcmu)
	return decoded
}
