package conn

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/relay-server"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"github.com/zaf/g711"
	"time"
)

type ConnectCallBack interface {
	GotVideoData(p []byte) (n int, err error)
	GotAudioData(p []byte) (n int, err error)
	RawCameraData() ([]byte, error)
	RawMicroData() ([]byte, error)
	AnswerForCallerCreated(string)
	OfferForCalleeCreated(string)
	EndCall(error)
	CallStart()
}

type NinjaRtpConn struct {
	status     webrtc.PeerConnectionState
	conn       *webrtc.PeerConnection
	videoTrack *webrtc.TrackLocalStaticSample
	audioTrack *webrtc.TrackLocalStaticSample
	callback   ConnectCallBack
	x264Writer *h264writer.H264Writer
	inVideoBuf chan *rtp.Packet
	inAudioBuf chan *rtp.Packet
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

type RawWriter struct {
	Writer func(p []byte) (n int, err error)
}

func (rw *RawWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}
	var rawData = make([]byte, len(p))
	copy(rawData, p)
	return rw.Writer(rawData)
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func createBasicConn(callback ConnectCallBack) (*NinjaRtpConn, error) {
	var conn = &NinjaRtpConn{
		status:     webrtc.PeerConnectionStateNew,
		inVideoBuf: make(chan *rtp.Packet, MaxConnBufferSize),
		inAudioBuf: make(chan *rtp.Packet, MaxConnBufferSize),
		callback:   callback,
	}
	var videoW = &RawWriter{
		Writer: callback.GotVideoData,
	}
	conn.x264Writer = h264writer.NewWith(videoW)
	var mediaEngine = &webrtc.MediaEngine{}

	var meErr = mediaEngine.RegisterCodec(relay.VideoParam, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		return nil, meErr
	}

	var acErr = mediaEngine.RegisterCodec(relay.AudioParam, webrtc.RTPCodecTypeAudio)
	if acErr != nil {
		return nil, acErr
	}

	var api = webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	var peerConnection, pcErr = api.NewPeerConnection(config)
	if pcErr != nil {
		return nil, pcErr
	}

	var videoOutputTrack, otErr = webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video-"+utils.MathRandAlpha(16),
		"video-"+utils.MathRandAlpha(16))
	if otErr != nil {
		return nil, otErr
	}
	var rtpSender, rsErr = peerConnection.AddTrack(videoOutputTrack)
	if rsErr != nil {
		return nil, rsErr
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				fmt.Println("======>>>video rtcp exit:", rtcpErr)
				return
			}
		}
	}()

	var audioOutTrack, aoErr = webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU},
		"audio-"+utils.MathRandAlpha(16),
		"audio-"+utils.MathRandAlpha(16))
	if aoErr != nil {
		return nil, aoErr
	}
	var audioRtpSender, arsErr = peerConnection.AddTrack(audioOutTrack)
	if arsErr != nil {
		return nil, arsErr
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := audioRtpSender.Read(rtcpBuf); rtcpErr != nil {
				fmt.Println("======>>>audio rtcp exit:", rtcpErr)
				return
			}
		}
	}()
	conn.audioTrack = audioOutTrack
	conn.videoTrack = videoOutputTrack
	conn.conn = peerConnection

	return conn, nil
}

func CreateCalleeRtpConn(offerStr string, callback ConnectCallBack) (*NinjaRtpConn, error) {
	fmt.Println("======>>>start to create answering conn")
	var nc, err = createBasicConn(callback)
	if err != nil {
		return nil, err
	}

	if err := nc.SetRemoteDesc(offerStr); err != nil {
		return nil, err
	}

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())
	go nc.readLocalVideo(iceConnectedCtx)
	go nc.readLocalAudio(iceConnectedCtx)
	go nc.consumeInVideo(iceConnectedCtx)
	go nc.consumeInAudio(iceConnectedCtx)

	nc.conn.OnTrack(nc.OnTrack)
	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		nc.status = s
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			iceConnectedCtxCancel()
			callback.CallStart()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			nc.callback.EndCall(fmt.Errorf("connection status:%s", s))
		}
	})

	var answer, errA = nc.createAnswerForCaller()
	if errA != nil {
		return nil, errA
	}
	nc.callback.AnswerForCallerCreated(answer)

	return nc, nil
}

func CreateCallerRtpConn(typ relay.SdpTyp, back ConnectCallBack) (*NinjaRtpConn, error) {
	fmt.Println("======>>>start to create calling conn")
	var nc, errConn = createBasicConn(back)
	if errConn != nil {
		return nil, errConn
	}

	nc.callback = back

	var offer, errOffer = nc.createOfferForRelay(typ)
	if errOffer != nil {
		return nil, errOffer
	}
	nc.callback.OfferForCalleeCreated(offer)

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())
	go nc.readLocalVideo(iceConnectedCtx)
	go nc.readLocalAudio(iceConnectedCtx)
	go nc.consumeInVideo(iceConnectedCtx)
	go nc.consumeInAudio(iceConnectedCtx)

	nc.conn.OnTrack(nc.OnTrack)
	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		nc.status = s
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			iceConnectedCtxCancel()
			back.CallStart()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			nc.callback.EndCall(fmt.Errorf("connection status:%s", s))
		}
	})

	return nc, nil
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func (nc *NinjaRtpConn) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	fmt.Printf("Track has started, of type %d: %s %s\n", track.PayloadType(), track.Codec().MimeType, track.Kind())

	for {
		pkt, _, readErr := track.ReadRTP()
		if readErr != nil {
			fmt.Println("========>>>read rtp err:", readErr)
			return
		}
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			nc.inAudioBuf <- pkt
		} else {
			nc.inVideoBuf <- pkt
		}
	}
}

func (nc *NinjaRtpConn) consumeInVideo(iceConnectedCtx context.Context) {
	<-iceConnectedCtx.Done()
	fmt.Println("======>>>start to reading remote video data")
	for {
		select {
		case pkt := <-nc.inVideoBuf:
			if err := nc.x264Writer.WriteRTP(pkt); err != nil {
				fmt.Println("========>>>write video rtp err:", err)
				nc.callback.EndCall(err)
				return
			}
		}
	}
}

func (nc *NinjaRtpConn) consumeInAudio(ceConnectedCtx context.Context) {
	<-ceConnectedCtx.Done()
	fmt.Println("======>>>start to reading remote audio data")
	for {
		select {
		case pkt := <-nc.inAudioBuf:
			var lpcm = g711.DecodeUlaw(pkt.Payload)
			var _, err = nc.callback.GotAudioData(lpcm)
			if err != nil {
				fmt.Println("========>>>write audio rtp err:", err)
				nc.callback.EndCall(err)
				return
			}
		}
	}
}

func (nc *NinjaRtpConn) readLocalVideo(iceConnectedCtx context.Context) {
	<-iceConnectedCtx.Done()
	fmt.Println("======>>> start to read video data:")
	for {
		var data, err = nc.callback.RawCameraData()
		if err != nil {
			fmt.Println("========>>>read local rtp err:", err)
			nc.callback.EndCall(err)
			return
		}
		if err := nc.videoTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
			fmt.Println("========>>>write to rtp err:", err)
			nc.callback.EndCall(err)
			return
		}
		//fmt.Println("======>>>camera data got:", len(data))
	}
}

func (nc *NinjaRtpConn) readLocalAudio(iceConnectedCtx context.Context) {
	<-iceConnectedCtx.Done()
	fmt.Println("======>>> start to read audio data:")
	for {
		var data, err = nc.callback.RawMicroData()
		if err != nil {
			fmt.Println("========>>>read local rtp err:", err)
			nc.callback.EndCall(err)
			return
		}
		fmt.Println("======>>>local audio data got: ", len(data))
		if err := nc.audioTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
			fmt.Println("========>>>write to rtp err:", err)
			nc.callback.EndCall(err)
			return
		}
	}
}

func (nc *NinjaRtpConn) Close() {

}

func (nc *NinjaRtpConn) IsConnected() bool {
	return nc.status == webrtc.PeerConnectionStateConnected
}

func (nc *NinjaRtpConn) SetRemoteDesc(des string) error {
	offer := relay.NinjaSdp{}
	var errEC = utils.Decode(des, &offer)
	if errEC != nil {
		return errEC
	}
	fmt.Println(offer.SDP)
	var pcErr = nc.conn.SetRemoteDescription(*offer.SDP)
	if pcErr != nil {
		return pcErr
	}
	return nil
}

func (nc *NinjaRtpConn) createAnswerForCaller() (string, error) {
	fmt.Println("======>>>creating answer for caller")
	var answer, errA = nc.conn.CreateAnswer(nil)
	if errA != nil {

		return "", errA
	}

	gatherComplete := webrtc.GatheringCompletePromise(nc.conn)

	if err := nc.conn.SetLocalDescription(answer); err != nil {
		return "", err
	}

	<-gatherComplete

	var sdp = &relay.NinjaSdp{
		Typ: relay.STAnswerToCaller,
		SID: "from-to-ninja-ids", //TODO:: refactor this later
		SDP: nc.conn.LocalDescription(),
	}

	var answerStr, err = utils.Encode(sdp)
	if err != nil {
		return "", err
	}
	fmt.Println(sdp.SDP)
	return answerStr, nil
}

func (nc *NinjaRtpConn) createOfferForRelay(typ relay.SdpTyp) (string, error) {
	fmt.Println("======>>>creating offer for callee")

	var offer, errOffer = nc.conn.CreateOffer(nil)
	if errOffer != nil {
		return "", errOffer
	}
	var gatheringWait = webrtc.GatheringCompletePromise(nc.conn)
	var err = nc.conn.SetLocalDescription(offer)
	if err != nil {
		return "", err
	}
	<-gatheringWait

	var sdp = &relay.NinjaSdp{
		Typ: typ,
		SID: "from-to-ninja-ids", //TODO:: refactor this later
		SDP: nc.conn.LocalDescription(),
	}
	var offerStr, errEN = utils.Encode(sdp)
	if errEN != nil {
		return "", errEN
	}
	fmt.Println(sdp.SDP)
	return offerStr, nil
}
