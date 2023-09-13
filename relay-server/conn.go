package relay

import (
	"fmt"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/webrtc/v3"
)

type Conn struct {
	conn *webrtc.PeerConnection

	audioTrack  *webrtc.TrackLocalStaticRTP
	audioReader *webrtc.RTPSender

	videoTrack  *webrtc.TrackLocalStaticRTP
	videoReader *webrtc.RTPSender

	status chan webrtc.ICEConnectionState
	answer *webrtc.SessionDescription

	errSig chan error
}

func createBasicConn(sid string) (*Conn, error) {
	var mediaEngine = &webrtc.MediaEngine{}

	var videoCodec = codec.NewRTPH264Codec(VideoRate)
	var meErr = mediaEngine.RegisterCodec(videoCodec.RTPCodecParameters, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		return nil, meErr
	}

	var audioCode = codec.NewRTPOpusCodec(AudioRate)
	var acErr = mediaEngine.RegisterCodec(audioCode.RTPCodecParameters, webrtc.RTPCodecTypeAudio)
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
		status:      make(chan webrtc.ICEConnectionState, 2),
		errSig:      make(chan error, 2),
	}

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		conn.status <- connectionState
	})

	return conn, nil
}

func (c *Conn) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	if c.status != nil {
		close(c.status)
		c.status = nil
	}
}

func (c *Conn) createAnswerForOffer(offer webrtc.SessionDescription) error {
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

func (c *Conn) monitor() {
	defer c.Close()
	for {
		select {
		case err := <-c.errSig:
			fmt.Println("conn failed:", err)
			return
		}
	}
}

func (c *Conn) audioReadingRtp() {

	rtcpBuf := make([]byte, 1500)
	for {
		if _, _, rtcpErr := c.audioReader.Read(rtcpBuf); rtcpErr != nil {
			c.errSig <- rtcpErr
			return
		}
	}
}

func (c *Conn) videoReadingRtp() {

	rtcpBuf := make([]byte, 1500)
	for {
		if _, _, rtcpErr := c.videoReader.Read(rtcpBuf); rtcpErr != nil {
			c.errSig <- rtcpErr
			return
		}
	}
}
