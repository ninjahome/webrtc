package main

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	"github.com/pion/mediadevices/pkg/codec/x264"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	"math/rand"
	"os"
)

func main() {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	offer := webrtc.SessionDescription{}

	internal.Decode(internal.MustReadStdin(), &offer)
	x264Params, err := x264.NewParams()
	internal.Must(err)
	x264Params.Preset = x264.PresetMedium
	x264Params.BitRate = 1_000_000 // 1mbps

	vp8Params, err := vpx.NewVP8Params()
	internal.Must(err)

	opusParams, err := opus.NewParams()
	internal.Must(err)
	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithVideoEncoders(&vp8Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)
	mediaEngine := webrtc.MediaEngine{}

	codecSelector.Populate(&mediaEngine)
	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))
	var peerConnection, peerErr = api.NewPeerConnection(config)
	internal.Must(peerErr)
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

	mediaStream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.FrameFormat = prop.FrameFormat(frame.FormatI420)
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
		},
		Audio: func(c *mediadevices.MediaTrackConstraints) {
		},
		Codec: codecSelector,
	})
	internal.Must(err)

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
		}
	})
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}
	})

	videoInputTrack := mediaStream.GetVideoTracks()[0].(*mediadevices.VideoTrack)
	defer videoInputTrack.Close()

	audioInputTrack := mediaStream.GetAudioTracks()[0].(*mediadevices.AudioTrack)
	defer audioInputTrack.Close()

	videoOutputTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
	internal.Must(err)
	videoRtpSender, videoTrackErr := peerConnection.AddTrack(videoOutputTrack)
	if videoTrackErr != nil {
		panic(videoTrackErr)
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := videoRtpSender.Read(rtcpBuf); rtcpErr != nil {
				panic(rtcpErr)
			}
		}
	}()

	audioOutputTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	internal.Must(err)
	audioRtpSender, err := peerConnection.AddTrack(audioOutputTrack)
	internal.Must(err)
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := audioRtpSender.Read(rtcpBuf); rtcpErr != nil {
				panic(rtcpErr)
			}
		}
	}()

	go func() {
		rtpReader, err := videoInputTrack.NewRTPReader(webrtc.MimeTypeVP8, rand.Uint32(), internal.MTU)
		internal.Must(err)

		<-iceConnectedCtx.Done()
		for {
			pkts, release, err := rtpReader.Read()
			internal.Must(err)
			for _, pkt := range pkts {
				if len(pkt.Payload) < 4 {
					fmt.Println("too short", pkt.String())
					continue
				}

				if err := videoOutputTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			}
			release()
		}
	}()

	go func() {
		rtpReader, err := audioInputTrack.NewRTPReader(webrtc.MimeTypeOpus, rand.Uint32(), internal.MTU)
		internal.Must(err)
		<-iceConnectedCtx.Done()
		for {
			pkts, release, err := rtpReader.Read()
			internal.Must(err)
			for _, pkt := range pkts {
				if err := audioOutputTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			}
			release()
		}

	}()

	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	<-gatherComplete

	fmt.Println(internal.Encode(*peerConnection.LocalDescription()))

	select {}

}
