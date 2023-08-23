package webrtcLib

import (
	"bytes"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"sync"
)

const (
	MaxBufferSize = 1 << 10
)

type AppInst struct {
	appLocker sync.RWMutex
	CallBack
	//queue        deque.Deque[[]byte]
	videoRawBuff chan []byte //deque.Deque[[]byte]
	p2pConn      *webrtc.PeerConnection
	builder      *samplebuilder.SampleBuilder
	x264Writer   *h264writer.H264Writer
}

var (
	startCode = []byte{0x00, 0x00, 0x00, 0x01}
	sCodeLen  = len(startCode)
)

const (
	H264TypMask = 0x1f
)

func h254Write(p []byte, callback func(typ int, h264data []byte)) (n int, err error) {
	if len(p) < 5 {
		fmt.Println("======>>>invalid rtp packets:", p)
		return 0, nil
	}

	var startIdx = bytes.Index(p, startCode)
	if startIdx != 0 {
		return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
	}

	var typ = int(p[sCodeLen] & H264TypMask)
	var origLen = len(p)
	p = p[sCodeLen:]
	if typ == 7 || typ == 8 {
		startIdx = bytes.Index(p, startCode)
		if startIdx < 0 {
			callback(typ, p)
			return len(p), nil
		}
		callback(typ, p[:startIdx])
		var nextStart = startIdx + sCodeLen
		var nextTyp = int(p[nextStart] & H264TypMask)
		p = p[nextStart:]
		callback(nextTyp, p)
		return origLen - sCodeLen, nil
	}

	if typ > 0 {
		//var dataLen = origLen - sCodeLen
		//fmt.Println("======>>> data len:", dataLen)
		//binary.LittleEndian.PutUint32(p[:sCodeLen], uint32(dataLen))
		callback(typ, p)
		if typ != 1 && typ != 5 {
			fmt.Println("==================>new type", typ)
		}
		return origLen, nil
	}

	return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
}
func (ai *AppInst) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}
	//fmt.Println("======>>>sample data:", hex.EncodeToString(p))
	var rawData = make([]byte, len(p))
	copy(rawData, p)
	//ai.videoRawBuff = append(ai.videoRawBuff, p...)
	return h254Write(rawData, ai.NewVideoData)
	//return len(p), nil
}

var _inst = &AppInst{}

type CallBack interface {
	NewVideoData(typ int, h264data []byte)
}

func (ai *AppInst) build(packets *rtp.Packet) error {
	//fmt.Println("======>>>packet type:", packets.String())
	return ai.x264Writer.WriteRTP(packets)
}

func (ai *AppInst) readingFromPeer() {
	defer fmt.Println("======>>> reading go thread exit")
	fmt.Println("======>>> start to read data from peer")
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
