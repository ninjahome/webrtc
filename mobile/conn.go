package webrtcLib

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"time"
)

type ConnectCallBack interface {
	GotVideoData(p []byte) (n int, err error)
	RawCameraData() ([]byte, error)
	RawMicroData() ([]byte, error)
	AnswerForCallerCreated(string)
	OfferForCalleeCreated(string)
	EndCall()
}

type NinjaConn struct {
	conn       *webrtc.PeerConnection
	videoTrack *webrtc.TrackLocalStaticSample
	audioTrack *webrtc.TrackLocalStaticSample
	callback   ConnectCallBack
	x264Writer *h264writer.H264Writer
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func (nc *NinjaConn) Write(p []byte) (n int, err error) {
	return nc.callback.GotVideoData(p)
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func (nc *NinjaConn) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	fmt.Printf("Track has started, of type %d: %s %s\n", track.PayloadType(), track.Codec().MimeType, track.Kind())
	if track.Kind() == webrtc.RTPCodecTypeAudio {
		return
	}
	for {
		pkt, _, readErr := track.ReadRTP()
		if readErr != nil {
			fmt.Println("========>>>read rtp err:", readErr)
			return
		}

		if err := nc.x264Writer.WriteRTP(pkt); err != nil {
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
			nc.callback.EndCall()
			return
		}
		if err := nc.videoTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
			fmt.Println("========>>>write to rtp err:", err)
			nc.callback.EndCall()
			return
		}
	}
}

func (nc *NinjaConn) readLocalAudio(iceConnectedCtx context.Context) {
	<-iceConnectedCtx.Done()
	for {
		var data, err = nc.callback.RawMicroData()
		if err != nil {
			fmt.Println("========>>>read local rtp err:", err)
			nc.callback.EndCall()
			return
		}
		if err := nc.audioTrack.WriteSample(media.Sample{Data: data, Duration: time.Second}); err != nil {
			fmt.Println("========>>>write to rtp err:", err)
			nc.callback.EndCall()
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

func (nc *NinjaConn) setRemoteDescription(des string) error {
	offer := webrtc.SessionDescription{}
	var errEC = utils.Decode(des, &offer)
	if errEC != nil {
		return errEC
	}
	var pcErr = nc.conn.SetRemoteDescription(offer)
	if pcErr != nil {
		return pcErr
	}
	return nil
}

func (nc *NinjaConn) createAnswerForCaller() (string, error) {
	var answer, errA = nc.conn.CreateAnswer(nil)
	if errA != nil {

		return "", errA
	}

	gatherComplete := webrtc.GatheringCompletePromise(nc.conn)

	if err := nc.conn.SetLocalDescription(answer); err != nil {
		return "", err
	}

	<-gatherComplete
	var answerStr, err = utils.Encode(*nc.conn.LocalDescription())
	if err != nil {
		return "", err
	}
	fmt.Println(answerStr)
	return answerStr, nil
}

func (nc *NinjaConn) createOfferForCallee() (string, error) {
	//
	//_, err := nc.conn.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	//if err != nil {
	//	return "", err
	//}
	//_, err = nc.conn.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	//if err != nil {
	//	return "", err
	//}

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
	var offerStr, errEN = utils.Encode(nc.conn.LocalDescription())
	if errEN != nil {
		return "", errEN
	}
	return offerStr, nil
}

/************************************************************************************************************
*
*
*
*
************************************************************************************************************/

func createBasicConn() (*NinjaConn, error) {
	var mediaEngine = &webrtc.MediaEngine{}
	var conn = &NinjaConn{}

	conn.x264Writer = h264writer.NewWith(conn)

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

	var api = webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
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
				fmt.Println("======>>>video rtcp exit:", rtcpErr)
				return
			}
		}
	}()

	//var audioOutTrack, aoErr = webrtc.NewTrackLocalStaticSample(audioCode.RTPCodecCapability, "audio", "ninja-audio")
	//if aoErr != nil {
	//	return nil, aoErr
	//}
	//var audioRtpSender, arsErr = peerConnection.AddTrack(audioOutTrack)
	//if arsErr != nil {
	//	return nil, arsErr
	//}
	//go func() {
	//	rtcpBuf := make([]byte, 1500)
	//	for {
	//		if _, _, rtcpErr := audioRtpSender.Read(rtcpBuf); rtcpErr != nil {
	//			fmt.Println("======>>>audio rtcp exit:", rtcpErr)
	//			return
	//		}
	//	}
	//}()
	//conn.audioTrack = audioOutTrack

	conn.videoTrack = videoOutputTrack
	conn.conn = peerConnection

	return conn, nil
}

func CreateConnectAsCallee(offerStr string, callback ConnectCallBack) (*NinjaConn, error) {
	var nc, err = createBasicConn()
	if err != nil {
		return nil, err
	}
	nc.callback = callback

	if err := nc.setRemoteDescription(offerStr); err != nil {
		return nil, err
	}

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())
	go nc.readLocalVideo(iceConnectedCtx)
	//go nc.readLocalAudio(iceConnectedCtx)

	nc.conn.OnTrack(nc.OnTrack)
	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			iceConnectedCtxCancel()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			nc.callback.EndCall()
		}
	})

	var answer, errA = nc.createAnswerForCaller()
	if errA != nil {
		return nil, errA
	}
	nc.callback.AnswerForCallerCreated(answer)

	return nc, nil
}

func CreateConnectionAsCaller(back ConnectCallBack) (*NinjaConn, error) {
	var nc, errConn = createBasicConn()
	if errConn != nil {
		return nil, errConn
	}

	nc.callback = back

	var offer, errOffer = nc.createOfferForCallee()
	if errOffer != nil {
		return nil, errOffer
	}
	nc.callback.OfferForCalleeCreated(offer)

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())
	go nc.readLocalVideo(iceConnectedCtx)
	//nc.conn.OnTrack(nc.OnTrack)
	nc.conn.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := track.Codec()
		fmt.Println("------>>>codec:", codec)

	})
	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			iceConnectedCtxCancel()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			nc.callback.EndCall()
		}
	})
	nc.conn.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed %s \n", connectionState.String())
	})

	return nc, nil
}
