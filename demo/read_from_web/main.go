package main

import (
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
	"os"
	"strings"
)

func main() {
	mediaEngine := &webrtc.MediaEngine{}

	var videoCodec = codec.NewRTPH264Codec(90000)
	var meErr = mediaEngine.RegisterCodec(videoCodec.RTPCodecParameters, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		internal.Must(meErr)
	}

	var audioCode = codec.NewRTPOpusCodec(48000)
	var acErr = mediaEngine.RegisterCodec(audioCode.RTPCodecParameters, webrtc.RTPCodecTypeAudio)
	if acErr != nil {
		internal.Must(meErr)
	}

	i := &interceptor.Registry{}

	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		panic(err)
	}
	i.Add(intervalPliFactory)

	if err = webrtc.RegisterDefaultInterceptors(mediaEngine, i); err != nil {
		panic(err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithInterceptorRegistry(i))

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	peerConnection, err := api.NewPeerConnection(config)
	internal.Must(err)

	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	} else if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}

	oggFile, err := oggwriter.New("output.ogg", 48000, 2)
	internal.Must(err)
	ivfFile, err := h264writer.New("offer.h264")
	internal.Must(err)

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := track.Codec()
		fmt.Println("------>>>codec:", codec)
		if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
			fmt.Println("Got Opus track, saving to disk as output.opus (48 kHz, 2 channels)")
			internal.SaveToDisk(oggFile, track)

		} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
			fmt.Println("Got H264 track, saving to disk as offer.h264")
			internal.SaveToDisk(ivfFile, track)
		}
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateConnected {
			fmt.Println("Ctrl+C the remote client to stop the demo")
		} else if connectionState == webrtc.ICEConnectionStateFailed {
			if closeErr := oggFile.Close(); closeErr != nil {
				panic(closeErr)
			}

			if closeErr := ivfFile.Close(); closeErr != nil {
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
	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	internal.Decode(internal.MustReadStdin(), &offer)

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(internal.Encode(*peerConnection.LocalDescription()))

	// Block forever
	select {}
}
