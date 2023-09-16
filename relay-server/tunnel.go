package relay

import (
	"context"
	"fmt"
	"github.com/pion/webrtc/v3"
	"strings"
)

const (
	VideoRate          = 90000
	AudioRate          = 48000
	NinjaAudioChannels = 1
)

var (
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	AudioParam = webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    AudioRate,
			Channels:     NinjaAudioChannels,
			SDPFmtpLine:  "minptime=10;useinbandfec=1",
			RTCPFeedback: nil},
		PayloadType: 111,
	}

	VideoParam = webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeH264,
			ClockRate:    VideoRate,
			Channels:     0,
			SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			RTCPFeedback: nil},
		PayloadType: 125,
	}
)

type Tunnel struct {
	TID string

	calleeWait context.Context
	calleeOk   context.CancelFunc

	callerConn *Conn
	calleeConn *Conn

	errSig chan error
}

func NewTunnel(sdp *NinjaSdp, tidRet chan string) (*Tunnel, *webrtc.SessionDescription, error) {

	fmt.Println("creating new tunnel:", sdp.SID)

	var ctx, cancel = context.WithCancel(context.Background())

	var t = &Tunnel{
		TID:    sdp.SID,
		errSig: make(chan error, 6),

		calleeWait: ctx,
		calleeOk:   cancel,
	}
	var c, err = newBasicConn(sdp.SID, t.errSig)
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
	go t.monitor(tidRet)
	return t, c.answer, nil
}

func (t *Tunnel) Close() {
	fmt.Println("tunnel is closing:", t.TID)
	if t.calleeConn != nil {
		t.calleeConn.Close()
		t.calleeConn = nil
	}
	if t.callerConn != nil {
		t.callerConn.Close()
		t.callerConn = nil
	}
}

func (t *Tunnel) UpdateTunnel(sdp *NinjaSdp) (*webrtc.SessionDescription, error) {

	var c, err = newBasicConn(sdp.SID, t.errSig)
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

	var codec = track.Codec()
	var local *webrtc.TrackLocalStaticRTP
	fmt.Println("caller's track success:", track.Codec().MimeType)

	<-t.calleeWait.Done()

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

	var codec = track.Codec()
	var local *webrtc.TrackLocalStaticRTP
	fmt.Println("callee 's track success", track.Codec().MimeType)
	t.calleeOk()

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

func (t *Tunnel) monitor(errTid chan string) {
	for {
		select {
		case err := <-t.errSig:
			fmt.Println("tunnel close by err:", err)
			t.Close()
			errTid <- t.TID
			return

		}
	}
}
