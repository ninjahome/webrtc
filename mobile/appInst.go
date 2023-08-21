package webrtcLib

import (
	"encoding/hex"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"io"
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
	builder      *samplebuilder.SampleBuilder
	x264Writer   *h264writer.H264Writer
	x264Reader   io.Reader
	//x264Reader *h264reader.H264Reader
}

var _inst = &AppInst{}

type CallBack interface {
	NewVideoData(h264data []byte)
}

func (ai *AppInst) build(packets *rtp.Packet) error {
	//ai.builder.Push(packets)
	//
	//for sample := ai.builder.Pop(); sample != nil; sample = ai.builder.Pop() {
	//	var rawData = make([]byte, len(sample.Data))
	//	fmt.Println("======>>>packet type:", sample.Duration)
	//	copy(rawData, sample.Data)
	//	_inst.videoRawBuff <- rawData
	//}
	//
	////fmt.Println("======>>>packet type:", packets.String())
	//return nil
	return ai.x264Writer.WriteRTP(packets)
}

func (ai *AppInst) readingFromPeer() {
	defer fmt.Println("======>>> reading go thread exit")
	fmt.Println("======>>> start to read data from peer")
	var buffer = make([]byte, 1<<10)
	for {
		var n, e = ai.x264Reader.Read(buffer)
		if e != nil {
			fmt.Println("======>>>x264Reader read err:", e)
			return
		}
		ai.NewVideoData(buffer[:n])
		fmt.Println("======>>>reader data:", hex.EncodeToString(buffer[:n]))
	}
	//for {
	//	var nal, err = ai.x264Reader.NextNAL()
	//	if err != nil {
	//		if err == io.EOF {
	//			fmt.Println("======>>>x264Reader read one data:", err)
	//			continue
	//		}
	//		fmt.Println("======>>>x264Reader read err:", err)
	//		return
	//	}
	//	fmt.Println("======>>>reader data:", nal.UnitType.String())
	//	ai.NewVideoData(nal.Data)
	//}
	//for {
	//	select {
	//	case data := <-ai.videoRawBuff:
	//		fmt.Println("======>>>sample data:", hex.EncodeToString(data))
	//		ai.NewVideoData(data)
	//	}
	//}
}

type RemoveRtpPayload func(*rtp.Packet) error

func createP2pConnect(offerStr string, callback RemoveRtpPayload) (*webrtc.PeerConnection, error) {

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

	var outputTrack, otErr = webrtc.NewTrackLocalStaticRTP(videoCodec.RTPCodecCapability, "video", "ninja-video")
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
			if err := callback(packets); err != nil {
				fmt.Println("========>>>send rtp err:", readErr)
				return
			}
			//fmt.Println("======>>>packet type:", rtp.String())
			//var rawData = make([]byte, len(rtp.Payload))
			//copy(rawData, rtp.Payload)
			//if err := callback(rawData); err != nil {
			//	fmt.Println("========>>>send rtp err:", readErr)
			//	return
			//}
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
