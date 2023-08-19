package webrtcLib

import (
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v3"
	"sync"
)

const (
	MaxBufferSize = 1 << 10
)

type AppInst struct {
	appLocker sync.RWMutex
	CallBack

	videoRawBuff chan []byte //deque.Deque[[]byte]
	p2pConn      *webrtc.PeerConnection
}

var _inst = &AppInst{}

type CallBack interface {
	NewVideoData(h264data []byte)
}

func (ai *AppInst) readingFromPeer() {
	defer fmt.Println("======>>> start to read data from peer")
	for bytes := range ai.videoRawBuff {
		ai.NewVideoData(bytes)
	}
	fmt.Println("======>>> reading go thread exit")
	return
}

type RemoveRtpPayload func([]byte) error

func createP2pConnect(offerStr string, callback RemoveRtpPayload) (*webrtc.PeerConnection, error) {

	var mediaEngine = &webrtc.MediaEngine{}

	var meErr = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		return nil, meErr
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

	var outputTrack, otErr = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "ninja")
	if otErr != nil {
		return nil, otErr
	}
	var rtpSender, rsErr = peerConnection.AddTrack(outputTrack)
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
	offer := webrtc.SessionDescription{}
	utils.Decode(offerStr, &offer)

	pcErr = peerConnection.SetRemoteDescription(offer)
	if pcErr != nil {
		return nil, pcErr
	}

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().MimeType)
		for {
			rtp, _, readErr := track.ReadRTP()
			if readErr != nil {
				fmt.Println("========>>>read rtp err:", readErr)
				return
			}
			//TODO::remove as callback
			if err := callback(rtp.Payload); err != nil {
				fmt.Println("========>>>send rtp err:", readErr)
				return
			}
		}
	})

	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			StopVideo()
		}
	})

	var answer, err = peerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	<-gatherComplete

	fmt.Println(utils.Encode(*peerConnection.LocalDescription()))

	return peerConnection, nil
}
