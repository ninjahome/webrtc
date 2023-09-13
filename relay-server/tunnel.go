package relay

import (
	"context"
	"fmt"
	"github.com/pion/webrtc/v3"
	"strings"
)

const (
	VideoRate = 90000
	AudioRate = 48000
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

type Tunnel struct {
	TID string

	calleeWait context.Context
	calleeOk   context.CancelFunc

	closeDelegate TunnelCloseCallBack

	callerConn *Conn
	calleeConn *Conn
}
type TunnelCloseCallBack func(string)

func NewTunnel(sdp *NinjaSdp, callback TunnelCloseCallBack) (*Tunnel, error) {

	var ctx, cancel = context.WithCancel(context.Background())

	var t = &Tunnel{
		TID: sdp.SID,

		calleeWait: ctx,
		calleeOk:   cancel,

		closeDelegate: callback,
	}
	var c, err = createBasicConn(sdp.SID)
	if err != nil {
		return nil, err
	}

	c.conn.OnTrack(t.OnCallerTrack)
	err = c.createAnswerForOffer(*sdp.SDP)
	if err != nil {
		c.Close()
		return nil, err
	}

	t.callerConn = c
	return t, nil
}

func (t *Tunnel) Close() {
	if t.calleeConn != nil {
		t.calleeConn.Close()

	}
	if t.callerConn != nil {
		t.callerConn.Close()
	}
	if t.closeDelegate != nil {
		t.closeDelegate(t.TID)
	}
}

func (t *Tunnel) UpdateCalleeSdp(sdp *NinjaSdp) (*webrtc.SessionDescription, error) {

	var c, err = createBasicConn(sdp.SID)
	if err != nil {
		return nil, err
	}
	c.conn.OnTrack(t.OnCalleeTrack)
	err = c.createAnswerForOffer(*sdp.SDP)
	if err != nil {
		c.Close()
		return nil, err
	}
	t.calleeConn = c
	return c.answer, nil
}

func relayRtp(track *webrtc.TrackRemote, audioTrack *webrtc.TrackLocalStaticRTP, videoTrack *webrtc.TrackLocalStaticRTP) error {
	var codec = track.Codec()

	if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
		if audioTrack == nil {
			return fmt.Errorf("audio track is nil")
		}
		for {
			rtp, _, readErr := track.ReadRTP()
			if readErr != nil {
				fmt.Println("read audio rtp err:", readErr)
				return readErr
			}

			if writeErr := audioTrack.WriteRTP(rtp); writeErr != nil {
				fmt.Println("write  audio rtp err:", writeErr)
				return writeErr
			}
		}
	} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
		if videoTrack == nil {
			return fmt.Errorf("video track is nil")
		}
		for {
			rtp, _, readErr := track.ReadRTP()
			if readErr != nil {
				fmt.Println("read video rtp err:", readErr)
				return readErr
			}

			if writeErr := videoTrack.WriteRTP(rtp); writeErr != nil {
				fmt.Println("write video rtp err:", writeErr)
				return writeErr
			}
		}
	}
	return fmt.Errorf("unknown track codec:%s", codec.MimeType)
}

func (t *Tunnel) OnCallerTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	<-t.calleeWait.Done()

	if err := relayRtp(track, t.calleeConn.audioTrack, t.calleeConn.videoTrack); err != nil {
		t.Close()
		return
	}
}

func (t *Tunnel) OnCalleeTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	t.calleeOk()
	if err := relayRtp(track, t.callerConn.audioTrack, t.callerConn.videoTrack); err != nil {
		t.Close()
		return
	}
}

func (t *Tunnel) CallerAnswerSdp() *webrtc.SessionDescription {
	return t.callerConn.answer
}
