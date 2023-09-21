package conn

import (
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
	EndCall(error)
	CallStart()
}

type NinjaRtpConn struct {
	status   webrtc.PeerConnectionState
	conn     *webrtc.PeerConnection
	hasVideo bool

	videoTrack *webrtc.TrackLocalStaticSample
	videoRtcp  *webrtc.RTPSender

	audioTrack *webrtc.TrackLocalStaticSample
	audioRtcp  *webrtc.RTPSender

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

func createBasicConn(hasVideo bool, callback ConnectCallBack) (*NinjaRtpConn, error) {
	var conn = &NinjaRtpConn{
		status:     webrtc.PeerConnectionStateNew,
		inVideoBuf: make(chan *rtp.Packet, MaxConnBufferSize),
		inAudioBuf: make(chan *rtp.Packet, MaxConnBufferSize),
		callback:   callback,
		hasVideo:   hasVideo,
	}
	var mediaEngine = &webrtc.MediaEngine{}
	if hasVideo {
		var videoW = &RawWriter{
			Writer: callback.GotVideoData,
		}
		conn.x264Writer = h264writer.NewWith(videoW)

		var meErr = mediaEngine.RegisterCodec(relay.VideoParam, webrtc.RTPCodecTypeVideo)
		if meErr != nil {
			return nil, meErr
		}
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

	if hasVideo {
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

		conn.videoRtcp = rtpSender
		conn.videoTrack = videoOutputTrack
	}

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
	conn.audioRtcp = audioRtpSender
	conn.audioTrack = audioOutTrack

	conn.conn = peerConnection

	return conn, nil
}

func readRtcp(reader *webrtc.RTPSender) {
	func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := reader.Read(rtcpBuf); rtcpErr != nil {
				fmt.Println("======>>>sender rtcp exit:", rtcpErr)
				return
			}
		}
	}()
}

func CreateCallerRtpConn(hasVideo bool, back ConnectCallBack) (*NinjaRtpConn, error) {
	fmt.Println("======>>>start to create calling conn")
	var nc, errConn = createBasicConn(hasVideo, back)
	if errConn != nil {
		return nil, errConn
	}

	nc.conn.OnTrack(nc.OnTrack)

	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		nc.status = s
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			nc.relayStart()
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

func (nc *NinjaRtpConn) GetOffer(typ relay.SdpTyp, sessionID string) (string, error) {
	var offer, errOffer = nc.createOfferForRelay(typ, sessionID)
	if errOffer != nil {
		return "", errOffer
	}

	return offer, nil
}

func (nc *NinjaRtpConn) relayStart() {

	if nc.hasVideo {
		go readRtcp(nc.videoRtcp)
		go nc.readLocalVideo()
		go nc.consumeInVideo()
	}

	go readRtcp(nc.audioRtcp)
	go nc.readLocalAudio()
	go nc.consumeInAudio()
}
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
		} else if nc.hasVideo {
			nc.inVideoBuf <- pkt
		} else {
			fmt.Println("======>>>unknown track:", track.Kind())
		}
	}
}

func (nc *NinjaRtpConn) consumeInVideo() {
	if !nc.hasVideo {
		fmt.Println("======>>>should not start video channel")
		return
	}
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

func (nc *NinjaRtpConn) consumeInAudio() {
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

func (nc *NinjaRtpConn) readLocalVideo() {
	if !nc.hasVideo {
		fmt.Println("======>>>should not input video data")
		return
	}

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

func (nc *NinjaRtpConn) readLocalAudio() {
	fmt.Println("======>>> start to read audio data:")
	for {
		var data, err = nc.callback.RawMicroData()
		if err != nil {
			fmt.Println("========>>>read local rtp err:", err)
			nc.callback.EndCall(err)
			return
		}
		//fmt.Println("======>>>local audio data got: ", len(data))
		if err := nc.audioTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
			fmt.Println("========>>>write to rtp err:", err)
			nc.callback.EndCall(err)
			return
		}
	}
}

func (nc *NinjaRtpConn) Close() {
	_ = nc.conn.Close()
	nc.status = webrtc.PeerConnectionStateClosed
	nc.callback.EndCall(fmt.Errorf("connection closed"))
	if nc.x264Writer != nil {
		_ = nc.x264Writer.Close()
	}
	if nc.inAudioBuf != nil {
		close(nc.inAudioBuf)
	}
	if nc.inVideoBuf != nil {
		close(nc.inVideoBuf)
	}
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
	//fmt.Println(offer.SDP)
	var pcErr = nc.conn.SetRemoteDescription(*offer.SDP)
	if pcErr != nil {
		return pcErr
	}
	return nil
}

func (nc *NinjaRtpConn) createOfferForRelay(typ relay.SdpTyp, sessionID string) (string, error) {
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
		SID: sessionID,
		SDP: nc.conn.LocalDescription(),
	}
	var offerStr, errEN = utils.Encode(sdp)
	if errEN != nil {
		return "", errEN
	}
	//fmt.Println(sdp.SDP)
	return offerStr, nil
}
