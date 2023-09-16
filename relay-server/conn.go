package relay

import (
	"fmt"
	"github.com/pion/webrtc/v3"
)

type Conn struct {
	conn *webrtc.PeerConnection

	audioTrack  *webrtc.TrackLocalStaticRTP
	audioReader *webrtc.RTPSender

	videoTrack  *webrtc.TrackLocalStaticRTP
	videoReader *webrtc.RTPSender

	status webrtc.ICEConnectionState
	answer *webrtc.SessionDescription

	errSig chan error
}

func newBasicConn(sid string, errCh chan error) (*Conn, error) {
	var mediaEngine = &webrtc.MediaEngine{}

	var meErr = mediaEngine.RegisterCodec(VideoParam, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		return nil, meErr
	}

	var acErr = mediaEngine.RegisterCodec(AudioParam, webrtc.RTPCodecTypeAudio)
	if acErr != nil {
		return nil, acErr
	}

	var api = webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	var peerConnection, pcErr = api.NewPeerConnection(config)
	if pcErr != nil {
		return nil, pcErr
	}

	var audioTrack, errAT = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", sid)
	if errAT != nil {
		return nil, errAT
	}
	var audioReader, errAr = peerConnection.AddTrack(audioTrack)
	if errAr != nil {
		return nil, errAr
	}

	var videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", sid)
	if err != nil {
		return nil, err
	}
	var videoReader, errVr = peerConnection.AddTrack(videoTrack)
	if errVr != nil {
		return nil, errVr
	}

	var conn = &Conn{
		conn:        peerConnection,
		audioTrack:  audioTrack,
		audioReader: audioReader,
		videoReader: videoReader,
		videoTrack:  videoTrack,
		errSig:      errCh,
	}

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Println("connection status changed:", connectionState.String())
		conn.status = connectionState
		if connectionState == webrtc.ICEConnectionStateConnected {
			conn.rtpStart()
		}
		if connectionState == webrtc.ICEConnectionStateFailed ||
			connectionState == webrtc.ICEConnectionStateDisconnected {
			conn.errSig <- fmt.Errorf("connection status %s", connectionState.String())
		}
	})
	fmt.Println("connection create success")
	return conn, nil
}

func (c *Conn) Close() {
	fmt.Println("connection is closing")
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Conn) createAnswerForOffer(offer webrtc.SessionDescription) error {
	fmt.Println("connection creating answer")
	if err := c.conn.SetRemoteDescription(offer); err != nil {
		return err
	}

	var answer, errAnswer = c.conn.CreateAnswer(nil)
	if errAnswer != nil {
		return errAnswer
	}
	var gatherComplete = webrtc.GatheringCompletePromise(c.conn)
	if err := c.conn.SetLocalDescription(answer); err != nil {
		return err
	}
	<-gatherComplete

	c.answer = c.conn.LocalDescription()

	return nil
}

func ReadingRtp(reader *webrtc.RTPSender, errCh chan error) {
	rtcpBuf := make([]byte, 1500)
	for {
		if _, _, rtcpErr := reader.Read(rtcpBuf); rtcpErr != nil {
			errCh <- rtcpErr
			return
		}
	}
}

func (c *Conn) rtpStart() {
	if c.audioReader != nil {
		fmt.Println("connection start to read audio rtcp")
		go ReadingRtp(c.audioReader, c.errSig)
	}
	if c.videoReader != nil {
		fmt.Println("connection start to read video rtcp")
		go ReadingRtp(c.videoReader, c.errSig)
	}
}
