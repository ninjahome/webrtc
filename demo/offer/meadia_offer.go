package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/x264"
	_ "github.com/pion/mediadevices/pkg/driver/camera" // This is required to register camera adapter
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
	"io"
	"math/rand"
	"net/http"
	"os"
)

func main() {
	offerAddr := flag.String("offer-address", ":50000", "Address that the Offer HTTP server is hosted on.")
	var peerConnection, err = webrtc.NewPeerConnection(
		webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		})
	//webrtc.Configuration{})
	internal.Must(err)
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()
	_, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	internal.Must(err)
	_, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	internal.Must(err)

	offer, err2 := peerConnection.CreateOffer(nil)
	internal.Must(err2)
	offerGatheringComplete := webrtc.GatheringCompletePromise(peerConnection)
	err = peerConnection.SetLocalDescription(offer)
	internal.Must(err)
	<-offerGatheringComplete

	fmt.Println(internal.Encode(peerConnection.LocalDescription()))

	http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
		sdp := &webrtc.SessionDescription{}
		body, _ := io.ReadAll(r.Body)
		fmt.Println(string(body))
		internal.Decode(string(body), sdp)
		if sdpErr := peerConnection.SetRemoteDescription(*sdp); sdpErr != nil {
			panic(sdpErr)
		}

	})
	go func() { panic(http.ListenAndServe(*offerAddr, nil)) }()

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
	oggFile, err := oggwriter.New("output.ogg", 48000, 2)
	internal.Must(err)
	ivfFile, err := ivfwriter.New("output.ivf")
	internal.Must(err)
	go func() {
		rtpReader, err := videoInputTrack.NewRTPReader(webrtc.MimeTypeH264, rand.Uint32(), internal.MTU)
		internal.Must(err)

		<-iceConnectedCtx.Done()
		for {
			pkts, release, err := rtpReader.Read()
			internal.Must(err)
			for _, pkt := range pkts {
				fmt.Println("------>>>writing video:", pkt.PayloadType)
				if err := videoOutputTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
				ivfFile.WriteRTP(pkt)
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
				fmt.Println("------>>>writing audio:", pkt.PayloadType)
				if err := audioOutputTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
				oggFile.WriteRTP(pkt)
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

	select {}
}
