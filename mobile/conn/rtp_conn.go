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
	EndCallByInnerErr(error)
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

	done     context.Context
	closeCtx context.CancelFunc
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
	var ctx, cl = context.WithCancel(context.Background())
	var conn = &NinjaRtpConn{
		status:     webrtc.PeerConnectionStateNew,
		inVideoBuf: make(chan *rtp.Packet, MaxConnBufferSize),
		inAudioBuf: make(chan *rtp.Packet, MaxConnBufferSize),
		callback:   callback,
		hasVideo:   hasVideo,
		done:       ctx,
		closeCtx:   cl,
	}

	var mediaEngine = &webrtc.MediaEngine{}
	if hasVideo {
		//fmt.Println("======>>>has video ability")
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
		//fmt.Println("======>>>creating video track")

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

func readRtcp(ctx context.Context, reader *webrtc.RTPSender) {

	rtcpBuf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			fmt.Println("======>>>sender rtcp for closing")
			return
		default:
			if _, _, rtcpErr := reader.Read(rtcpBuf); rtcpErr != nil {
				fmt.Println("======>>>sender rtcp exit:", rtcpErr)
				return
			}
		}
	}
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
			nc.disconnectedByError(fmt.Errorf("connection status:%s", s))
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

func (nc *NinjaRtpConn) disconnectedByError(err error) {
	if nc.callback == nil {
		return
	}
	nc.callback.EndCallByInnerErr(err)
	nc.Close()
	nc.callback = nil
}

func (nc *NinjaRtpConn) Close() {
	_ = nc.conn.Close()
	nc.closeCtx()

	if nc.x264Writer != nil {
		_ = nc.x264Writer.Close()
		nc.x264Writer = nil
	}
	if nc.inAudioBuf != nil {
		close(nc.inAudioBuf)
		nc.inAudioBuf = nil
	}
	if nc.inVideoBuf != nil {
		close(nc.inVideoBuf)
		nc.inVideoBuf = nil
	}
}

func (nc *NinjaRtpConn) GetOffer(typ relay.SdpTyp, sessionID string) (string, error) {
	var offer, errOffer = nc.createOfferForRelay(typ, sessionID)
	if errOffer != nil {
		return "", errOffer
	}
	return offer, nil
}

func (nc *NinjaRtpConn) relayStart() {

	if nc.hasVideo {
		go readRtcp(nc.done, nc.videoRtcp)
		go nc.readLocalVideo()
		go nc.consumeInVideo()
	}

	go readRtcp(nc.done, nc.audioRtcp)
	go nc.readLocalAudio()
	go nc.consumeInAudio()
}
func (nc *NinjaRtpConn) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	fmt.Printf("Track has started, of type %d: %s %s\n", track.PayloadType(), track.Codec().MimeType, track.Kind())

	for {
		select {
		case <-nc.done.Done():
			fmt.Println("========>>>tack exit for closing")
			return
		default:
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
				fmt.Println("========>>>consume video rtp err:", err)
				nc.disconnectedByError(err)
				return
			}

		case <-nc.done.Done():
			fmt.Println("========>>>consume video exit for closing")
			return
		}
	}
}

func (nc *NinjaRtpConn) consumeInAudio() {
	fmt.Println("======>>>start to reading remote audio data")

	for {
		select {
		case pkt := <-nc.inAudioBuf:
			var lpcm = g711.DecodeUlaw(pkt.Payload)
			if nc.callback == nil {
				fmt.Println("======>>>[consumeInAudio] connection closed")
				return
			}
			var _, err = nc.callback.GotAudioData(lpcm)
			if err != nil {
				fmt.Println("========>>>write audio rtp err:", err)
				nc.disconnectedByError(err)
				return
			}
		case <-nc.done.Done():
			fmt.Println("========>>>consume audio exit for closing")
			return
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
		select {
		case <-nc.done.Done():
			fmt.Println("========>>>read local video exit for closing")
			return
		default:
			if nc.callback == nil {
				fmt.Println("======>>>[readLocalVideo] connection closed")
				return
			}
			var data, err = nc.callback.RawCameraData()
			if err != nil {
				fmt.Println("========>>>read local video err:", err)
				nc.disconnectedByError(err)
				return
			}
			if err := nc.videoTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
				fmt.Println("========>>>write local video to peer err:", err)
				nc.disconnectedByError(err)
				return
			}
		}
	}
}

func (nc *NinjaRtpConn) readLocalAudio() {
	fmt.Println("======>>> start to read audio data:")
	for {
		select {
		case <-nc.done.Done():
			fmt.Println("========>>>read local video exit for closing ")
			return
		default:
			if nc.callback == nil {
				fmt.Println("======>>>[readLocalAudio] connection closed")
				return
			}
			var data, err = nc.callback.RawMicroData()
			if err != nil {
				fmt.Println("========>>>read local rtp err:", err)
				nc.disconnectedByError(err)
				return
			}
			//fmt.Println("======>>>local audio data got: ", len(data))
			if err := nc.audioTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
				fmt.Println("========>>>write to rtp err:", err)
				nc.disconnectedByError(err)
				return
			}
		}
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
