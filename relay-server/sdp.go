package relay

import (
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

func (sdp *NinjaSdp) String() string {
	var s = "\nsid\t:" + sdp.SID
	s += "\ntype\t:" + sdp.Typ.String()
	s += "\nwebrtc sdp\t:" + sdp.SDP.SDP
	return s
}
