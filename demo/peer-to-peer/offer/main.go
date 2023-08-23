package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/x264"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
)

func main() {
	offerAddr := flag.String("offer-address", ":50000", "Address that the Offer HTTP server is hosted on.")
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	m := &webrtc.MediaEngine{}
	x264Params, errX264 := x264.NewParams()
	internal.Must(errX264)
	x264Params.BitRate = 1_000_1000

	opusParams, errOpus := opus.NewParams()
	internal.Must(errOpus)
	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)
	codecSelector.Populate(m)
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	var peerConnection, err = api.NewPeerConnection(config)
	internal.Must(err)
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()
	//_, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	//internal.Must(err)

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
				panic(rtcpErr)
			}
		}
	}()

	_, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	internal.Must(err)

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

	//for _, track := range mediaStream.GetTracks() {
	//	track.OnEnded(func(err error) {
	//		fmt.Printf("Track (ID: %s) ended with error: %v\n",
	//			track.ID(), err)
	//	})
	//
	//	_, err := peerConnection.AddTransceiverFromTrack(track,
	//		webrtc.RTPTransceiverInit{
	//			Direction: webrtc.RTPTransceiverDirectionSendrecv,
	//		},
	//	)
	//	internal.Must(err)
	//}

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

	var oggFile, oggErr = oggwriter.New("output.ogg", 48000, 2)
	internal.Must(oggErr)
	var h264File, ivfErr = h264writer.New("offer.h264")
	internal.Must(ivfErr)

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := track.Codec()
		fmt.Println("------>>>codec:", codec)
		if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
			internal.SaveToDisk(oggFile, track)
		} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
			internal.SaveToDisk(h264File, track)
		}
	})
	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())
	videoInputTrack := mediaStream.GetVideoTracks()[0].(*mediadevices.VideoTrack)
	defer videoInputTrack.Close()

	go func() {
		rtpReader, err := videoInputTrack.NewRTPReader(webrtc.MimeTypeH264, rand.Uint32(), internal.MTU)
		internal.Must(err)

		<-iceConnectedCtx.Done()
		for {
			pkts, release, err := rtpReader.Read()
			internal.Must(err)
			for _, pkt := range pkts {
				//fmt.Println("======>>>", hex.EncodeToString(pkt.Payload))
				if err := videoOutputTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			}
			release()
		}
	}()

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateConnected {
			fmt.Println("Ctrl+C the remote client to stop the demo")
		} else if connectionState == webrtc.ICEConnectionStateFailed {
			if closeErr := oggFile.Close(); closeErr != nil {
				panic(closeErr)
			}

			if closeErr := h264File.Close(); closeErr != nil {
				panic(closeErr)
			}

			fmt.Println("Done writing media files")

			// Gracefully shutdown the peer connection
			if closeErr := peerConnection.Close(); closeErr != nil {
				panic(closeErr)
			}

			os.Exit(0)
		}
	})
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			iceConnectedCtxCancel()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}
	})

	select {}
}
