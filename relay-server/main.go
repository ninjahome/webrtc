package main

import (
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/webrtc/v3"
	"io"
	"net/http"
	"strings"
)

const (
	STStartCall SdpTyp = iota + 1
	STAnswerCaller
	STStartCalled
	STAnswerCallee
)

type SdpTyp int8

func (t SdpTyp) String() string {
	switch t {
	case STStartCall:
		return "start_call"
	case STAnswerCaller:
		return "answer_caller"
	case STStartCalled:
		return "callee_start"
	case STAnswerCallee:
		return "answer_callee"
	}

	return "unknown"
}

const (
	VideoRate = 90000
	AudioRate = 48000
)

var (
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

type NinjaSdp struct {
	Typ SdpTyp
	SDP *webrtc.SessionDescription
}

type RelayServer struct {
	sdpSig       chan *NinjaSdp
	errSig       chan error
	callerStatus chan webrtc.ICEConnectionState

	callerConn *webrtc.PeerConnection
	calleeConn *webrtc.PeerConnection
}

func (rs *RelayServer) sdpListen() {

	//TODO::refactor sdp service

	for typ := STStartCall; typ <= STAnswerCallee; typ++ {
		var t = typ
		http.HandleFunc("/sdp/"+typ.String(), func(w http.ResponseWriter, r *http.Request) {
			s := &webrtc.SessionDescription{}
			body, _ := io.ReadAll(r.Body)
			if err := utils.Decode(string(body), s); err != nil {
				panic(err)
			}
			rs.sdpSig <- &NinjaSdp{
				Typ: t,
				SDP: s,
			}
		})
	}

	go func() { panic(http.ListenAndServe(":50000", nil)) }()
}

func setSdp(conn *webrtc.PeerConnection, sdp *webrtc.SessionDescription) error {
	if conn == nil {
		fmt.Println("caller connection is nil when set sdp")
		return fmt.Errorf("caller connection is nil")
	}
	var sdpErr = conn.SetRemoteDescription(*sdp)
	if sdpErr != nil {
		fmt.Println("set sdp for caller err:", sdpErr)
		return sdpErr
	}

	return nil
}

func (rs *RelayServer) monitor() {

	defer rs.Close()

	for {
		select {
		case err := <-rs.errSig:
			fmt.Println("relay server quit by err:", err)
			return

		case status := <-rs.callerStatus:
			if status == webrtc.ICEConnectionStateFailed {
				fmt.Println("relay quit by caller connection failed")
				return
			}

		case sdp := <-rs.sdpSig:

			var sdpErr error
			switch sdp.Typ {
			case STStartCall:
				sdpErr = rs.prepareInCall(sdp.SDP)
			}
			if sdpErr != nil {
				return
			}
		}
	}
}

func (rs *RelayServer) createAnswer() (*webrtc.SessionDescription, error) {
	var answer, errAnswer = rs.callerConn.CreateAnswer(nil)
	if errAnswer != nil {
		return nil, errAnswer
	}
	var gatherComplete = webrtc.GatheringCompletePromise(rs.callerConn)
	if err := rs.callerConn.SetLocalDescription(answer); err != nil {
		return nil, err
	}
	<-gatherComplete

	return rs.callerConn.LocalDescription(), nil

}

func (rs *RelayServer) prepareInCall(offer *webrtc.SessionDescription) error {
	if err := rs.createBasicConn(); err != nil {
		return err
	}

	if err := rs.callerConn.SetRemoteDescription(*offer); err != nil {
		return err
	}

	var sdp, errAnswer = rs.createAnswer()
	if errAnswer != nil {
		return errAnswer
	}

	var str, err = utils.Encode(sdp)
	if err != nil {
		return err
	}

	//TODO:: send it to caller

	fmt.Println()
	fmt.Println(str)
	fmt.Println()

	return nil
}
func (rs *RelayServer) createBasicConn() error {
	var mediaEngine = &webrtc.MediaEngine{}

	var videoCodec = codec.NewRTPH264Codec(VideoRate)
	var meErr = mediaEngine.RegisterCodec(videoCodec.RTPCodecParameters, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		return meErr
	}

	var audioCode = codec.NewRTPOpusCodec(AudioRate)
	var acErr = mediaEngine.RegisterCodec(audioCode.RTPCodecParameters, webrtc.RTPCodecTypeAudio)
	if acErr != nil {
		return acErr
	}

	var api = webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	var peerConnection, pcErr = api.NewPeerConnection(config)
	if pcErr != nil {
		return pcErr
	}

	if _, err := peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}
	var outputTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "ninja-stream-echo")
	if err != nil {
		return err
	}
	var rtpSender, errAdd = peerConnection.AddTrack(outputTrack)
	if errAdd != nil {
		return errAdd
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				rs.errSig <- rtcpErr
				return
			}
		}
	}()

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {

		var codec = track.Codec()

		if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
			//TODO:: audio channel to be done later
		} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
			for {
				rtp, _, readErr := track.ReadRTP()
				if readErr != nil {
					rs.errSig <- readErr
					return
				}

				if writeErr := outputTrack.WriteRTP(rtp); writeErr != nil {
					rs.errSig <- writeErr
					return
				}
			}
		}
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		rs.callerStatus <- connectionState
	})
	rs.callerConn = peerConnection

	return nil
}

func (rs *RelayServer) Close() {
}

func main() {
	var rs = &RelayServer{
		sdpSig:       make(chan *NinjaSdp, 2),
		errSig:       make(chan error, 2),
		callerStatus: make(chan webrtc.ICEConnectionState, 3),
	}

	rs.sdpListen()
	rs.monitor()
}
