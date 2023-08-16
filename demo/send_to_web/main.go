package main

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/x264"      // This is required to use H264 video encoder
	_ "github.com/pion/mediadevices/pkg/driver/camera" // This is required to register camera adapter
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	"math/rand"
	"os"
)

func main() {

	x264Params, err := x264.NewParams()
	internal.Must(err)
	x264Params.Preset = x264.PresetMedium
	x264Params.BitRate = 1_000_000 // 1mbps

	opusParams, err := opus.NewParams()
	if err != nil {
		panic(err)
	}
	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)

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

	videoInputTrack := mediaStream.GetVideoTracks()[0].(*mediadevices.VideoTrack)
	defer videoInputTrack.Close()

	audioInputTrack := mediaStream.GetAudioTracks()[0].(*mediadevices.AudioTrack)
	defer audioInputTrack.Close()

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	internal.Must(err)
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

	videoOutputTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	internal.Must(err)
	videoRtpSender, videoTrackErr := peerConnection.AddTrack(videoOutputTrack)
	if videoTrackErr != nil {
		panic(videoTrackErr)
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := videoRtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
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
				return
			}
		}
	}()

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	go func() {
		rtpReader, err := videoInputTrack.NewRTPReader(webrtc.MimeTypeH264, rand.Uint32(), internal.MTU)
		internal.Must(err)

		<-iceConnectedCtx.Done()
		for {
			pkts, release, err := rtpReader.Read()
			internal.Must(err)
			for _, pkt := range pkts {
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

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
		}
	})
	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

		if s == webrtc.PeerConnectionStateFailed {
			// Wait until PeerConnection has had no network activity for 30 seconds or another failure. It may be reconnected using an ICE Restart.
			// Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
			// Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}
	})

	offer := webrtc.SessionDescription{}
	internal.Decode(internal.MustReadStdin(), &offer)
	// Set the remote SessionDescription
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(internal.Encode(*peerConnection.LocalDescription()))
	select {}
}
