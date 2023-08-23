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
)

type ConnectCallBack interface {
	GotRtp(*rtp.Packet) error
	StatusChanged(bool)
	RawCameraData() ([]byte, error)
}

func createP2pConnect(offerStr string, callback ConnectCallBack) (*webrtc.PeerConnection, error) {

	var mediaEngine = &webrtc.MediaEngine{}

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

	var videoOutputTrack, otErr = webrtc.NewTrackLocalStaticRTP(videoCodec.RTPCodecCapability, "video", "ninja-video")
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

	offer := webrtc.SessionDescription{}
	utils.Decode(offerStr, &offer)

	pcErr = peerConnection.SetRemoteDescription(offer)
	if pcErr != nil {
		return nil, pcErr
	}

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
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
			if err := callback.GotRtp(packets); err != nil {
				fmt.Println("========>>>send rtp err:", readErr)
				return
			}
		}
	})
	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	go func() {
		<-iceConnectedCtx.Done()
		//for {
		//	//var pkt, err = callback.LocalRtp()
		//	//if err != nil {
		//	//	fmt.Println("========>>>read local rtp err:", err)
		//	//	callback.StatusChanged(true)
		//	//	return
		//	//}
		//	//if err := videoOutputTrack.WriteRTP(pkt); err != nil {
		//	//	fmt.Println("========>>>write to rtp err:", err)
		//	//	callback.StatusChanged(true)
		//	//	return
		//	//}
		//}
	}()
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
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
