package relay

import (
	"fmt"
	"github.com/pion/webrtc/v3"
)

type SdpTyp int8

const (
	STCallerOffer SdpTyp = iota + 1
	STAnswerToCaller
	STCalleeOffer
	STAnswerToCallee
)

func (t SdpTyp) String() string {
	switch t {
	case STCallerOffer:
		return "caller_offer"
	case STAnswerToCaller:
		return "answer_caller"
	case STCalleeOffer:
		return "callee_offer"
	case STAnswerToCallee:
		return "answer_callee"
	}

	return "unknown"
}

type NinjaSdp struct {
	Typ SdpTyp
	SID string
	SDP *webrtc.SessionDescription
}

func setSdp(conn *webrtc.PeerConnection, sdp *webrtc.SessionDescription) error {
	if conn == nil {
		fmt.Println("caller connection is nil when set sdp")
		return fmt.Errorf("caller connection is nil")
	}
	var sdpErr = conn.SetRemoteDescription(*sdp)
	if sdpErr != nil {
		fmt.Println("set sdp for caller err:", sdpErr)
		return sdpErr
	}

	return nil
}
