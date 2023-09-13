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

func NewTunnel(sdp *NinjaSdp, callback TunnelCloseCallBack) (*Tunnel, *webrtc.SessionDescription, error) {

	fmt.Println("creating new tunnel:", sdp.SID)

	var ctx, cancel = context.WithCancel(context.Background())

	var t = &Tunnel{
		TID: sdp.SID,

		calleeWait: ctx,
		calleeOk:   cancel,

		closeDelegate: callback,
	}
	var c, err = newBasicConn(sdp.SID)
	if err != nil {
		fmt.Println("[NewTunnel] create basic connection err:", err)
		return nil, nil, err
	}

	c.conn.OnTrack(t.OnCallerTrack)
	err = c.createAnswerForOffer(*sdp.SDP)
	if err != nil {
		fmt.Println("[NewTunnel] create answer for caller err:", err)
		c.Close()
		return nil, nil, err
	}

	t.callerConn = c
	fmt.Println("create new connection for caller success!")
	return t, c.answer, nil
}

func (t *Tunnel) Close() {
	fmt.Println("tunnel is closing:", t.TID)
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

func (t *Tunnel) UpdateTunnel(sdp *NinjaSdp) (*webrtc.SessionDescription, error) {

	var c, err = newBasicConn(sdp.SID)
	if err != nil {
		fmt.Println("[UpdateTunnel] create connection for callee err:", err)
		return nil, err
	}

	c.conn.OnTrack(t.OnCalleeTrack)
	err = c.createAnswerForOffer(*sdp.SDP)
	if err != nil {
		fmt.Println("[UpdateTunnel] create answer for callee err:", err)
		c.Close()
		return nil, err
	}
	t.calleeConn = c
	fmt.Println("update tunnel success!")
	return c.answer, nil
}

func relayRtp(remote *webrtc.TrackRemote, local *webrtc.TrackLocalStaticRTP) error {

	if local == nil {
		return fmt.Errorf("local track is nil")
	}
	fmt.Println("start to relay track")
	for {
		rtp, _, readErr := remote.ReadRTP()
		if readErr != nil {
			fmt.Println("read audio rtp err:", readErr)
			return readErr
		}

		if writeErr := local.WriteRTP(rtp); writeErr != nil {
			fmt.Println("write rtp err:", writeErr)
			return writeErr
		}
	}
}

func (t *Tunnel) OnCallerTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	defer t.Close()

	<-t.calleeWait.Done()

	var codec = track.Codec()
	var local *webrtc.TrackLocalStaticRTP
	fmt.Println("caller's track success:", track.Codec().MimeType)

	if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
		local = t.calleeConn.audioTrack
	} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
		local = t.calleeConn.videoTrack
	} else {
		fmt.Println("unknown codec of track:", codec.MimeType)
		return
	}

	var err = relayRtp(track, local)
	if err != nil {
		fmt.Println("caller's track failed:", err, track.Codec().MimeType)
		return
	}
}

func (t *Tunnel) OnCalleeTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	t.calleeOk()

	var codec = track.Codec()
	var local *webrtc.TrackLocalStaticRTP
	fmt.Println("callee 's track success", track.Codec().MimeType)

	if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
		local = t.callerConn.audioTrack
	} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
		local = t.callerConn.videoTrack
	} else {
		fmt.Println("unknown codec of track:", codec.MimeType)
		return
	}
	var err = relayRtp(track, local)
	if err != nil {
		fmt.Println("callee 's track failed:", err, track.Codec().MimeType)
		return
	}
}

func (t *Tunnel) monitor() {
	for {
		select {

		case err := <-t.callerConn.errSig:
			fmt.Println("tunnel close for caller's connection err:", err)
			t.Close()
			return

		case err := <-t.calleeConn.errSig:
			fmt.Println("tunnel close for callee 's connection err:", err)
			t.Close()
			return
		}
	}
}
