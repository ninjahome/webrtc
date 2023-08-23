package webrtcLib

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"time"
)

type ConnectCallBack interface {
	GotRtp(*rtp.Packet) error
	StatusChanged(bool)
	RawCameraData() ([]byte, error)
}

type NinjaConn struct {
	conn       *webrtc.PeerConnection
	videoTrack *webrtc.TrackLocalStaticSample
	audioTrack *webrtc.TrackLocalStaticSample
	callback   ConnectCallBack
}

func createBasicConn() (*NinjaConn, error) {
	var mediaEngine = &webrtc.MediaEngine{}
	var conn = &NinjaConn{}
	var videoCodec = codec.NewRTPH264Codec(90000)
	var meErr = mediaEngine.RegisterCodec(videoCodec.RTPCodecParameters, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		return nil, meErr
	}

	var audioCode = codec.NewRTPOpusCodec(48000)
	var acErr = mediaEngine.RegisterCodec(audioCode.RTPCodecParameters, webrtc.RTPCodecTypeAudio)
	if acErr != nil {
		return nil, acErr
	}

	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, i); err != nil {
		return nil, err
	}
	var intervalPliFactory, ipErr = intervalpli.NewReceiverInterceptor()
	if ipErr != nil {
		return nil, ipErr
	}
	i.Add(intervalPliFactory)

	var api = webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithInterceptorRegistry(i))

	var peerConnection, pcErr = api.NewPeerConnection(config)
	if pcErr != nil {
		return nil, pcErr
	}
	var videoOutputTrack, otErr = webrtc.NewTrackLocalStaticSample(videoCodec.RTPCodecCapability, "video", "ninja-video")
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
				return
			}
		}
	}()

	conn.conn = peerConnection
	conn.videoTrack = videoOutputTrack

	var audioOutTrack, aoErr = webrtc.NewTrackLocalStaticRTP(audioCode.RTPCodecCapability, "audio", "ninja-audio")
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
				return
			}
		}
	}()

	return conn, nil
}
func (nc *NinjaConn) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	fmt.Printf("Track has started, of type %d: %s %s\n", track.PayloadType(), track.Codec().MimeType, track.Kind())
	if track.Kind() == webrtc.RTPCodecTypeAudio {
		return
	}
	for {
		packets, _, readErr := track.ReadRTP()
		if readErr != nil {
			fmt.Println("========>>>read rtp err:", readErr)
			return
		}
		if err := nc.callback.GotRtp(packets); err != nil {
			fmt.Println("========>>>send rtp err:", readErr)
			return
		}
	}
}
func (nc *NinjaConn) readLocalVideo(iceConnectedCtx context.Context) {
	<-iceConnectedCtx.Done()
	for {
		var data, err = nc.callback.RawCameraData()
		if err != nil {
			fmt.Println("========>>>read local rtp err:", err)
			nc.callback.StatusChanged(true)
			return
		}
		if err := nc.videoTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
			fmt.Println("========>>>write to rtp err:", err)
			nc.callback.StatusChanged(true)
			return
		}
	}
}
func (nc *NinjaConn) Close() {

}

func (nc *NinjaConn) IsConnected() bool {
	if nc.conn == nil {
		return false
	}
	return nc.conn.ConnectionState() != webrtc.PeerConnectionStateConnected
}

func createP2pConnect(offerStr string, callback ConnectCallBack) (*NinjaConn, error) {
	var nc, err = createBasicConn()
	if err != nil {
		return nil, err
	}
	nc.callback = callback

	offer := webrtc.SessionDescription{}
	var errEC = utils.Decode(offerStr, &offer)
	if errEC != nil {
		return nil, errEC
	}
	var pcErr = nc.conn.SetRemoteDescription(offer)
	if pcErr != nil {
		return nil, pcErr
	}

	nc.conn.OnTrack(nc.OnTrack)

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	go nc.readLocalVideo(iceConnectedCtx)

	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			callback.StatusChanged(true)
			iceConnectedCtxCancel()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			callback.StatusChanged(false)
		}
	})

	var answer, errA = nc.conn.CreateAnswer(nil)
	if errA != nil {
		return nil, errA
	}

	gatherComplete := webrtc.GatheringCompletePromise(nc.conn)

	if err = nc.conn.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	<-gatherComplete

	fmt.Println(utils.Encode(*nc.conn.LocalDescription()))

	return nc, nil
}

func createOfferConnect(remoteAnswer chan string, back CallBack) (*NinjaConn, error) {
	return createBasicConn()
}
