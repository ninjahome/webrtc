package main

import (
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/ninjahome/webrtc/relay-server"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/h264writer"
	"os"
	"strings"
)

func main() {
	mediaEngine := &webrtc.MediaEngine{}

	var meErr = mediaEngine.RegisterCodec(relay.VideoParam, webrtc.RTPCodecTypeVideo)
	if meErr != nil {
		internal.Must(meErr)
	}

	var acErr = mediaEngine.RegisterCodec(relay.AudioParam, webrtc.RTPCodecTypeAudio)
	if acErr != nil {
		internal.Must(meErr)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	var peerConnection, errPeer = api.NewPeerConnection(config)
	internal.Must(errPeer)

	var audioTrack, errAT = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU},
		"audio", "sid")
	if errAT != nil {
		internal.Must(errAT)
	}
	var audioReader, errAr = peerConnection.AddTrack(audioTrack)
	if errAr != nil {
		internal.Must(errAr)
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := audioReader.Read(rtcpBuf); rtcpErr != nil {
				internal.Must(rtcpErr)
			}
		}
	}()

	var videoTrack, errV = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "sid")
	if errV != nil {
		internal.Must(errV)
	}
	var videoReader, errVr = peerConnection.AddTrack(videoTrack)
	if errVr != nil {
		internal.Must(errVr)
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := videoReader.Read(rtcpBuf); rtcpErr != nil {
				internal.Must(rtcpErr)
			}
		}
	}()

	var wavFile, err = os.OpenFile("output.wav", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	internal.Must(err)
	ivfFile, err := h264writer.New("offer.h264")
	internal.Must(err)

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := track.Codec()
		fmt.Println("------>>>codec:", codec)
		if strings.EqualFold(codec.MimeType, webrtc.MimeTypePCMU) {
			fmt.Println("Got MimeTypePCMU track, saving to disk as output.opus (48 kHz, 2 channels)")
			for {
				rtpPacket, _, err := track.ReadRTP()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Println("------>>>writing rtpPacket: \n ", rtpPacket.String())
				err = audioTrack.WriteRTP(rtpPacket)
				if err != nil {
					internal.Must(err)
				}

				//var lpcm = g711.DecodeUlaw(rtpPacket.Payload)
				//
				//fmt.Println()
				//fmt.Println(hex.EncodeToString(rtpPacket.Payload))
				//fmt.Println()
				//_, err = wavFile.Write(lpcm)
				//if err != nil {
				//	panic(err)
				//}
			}

		} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
			fmt.Println("Got H264 track, saving to disk as offer.h264")
			//internal.SaveToDisk(ivfFile, track)
			for {
				rtpPacket, _, err := track.ReadRTP()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				fmt.Println("------>>>writing rtpPacket: \n ", rtpPacket.String())

				err = videoTrack.WriteRTP(rtpPacket)
				if err != nil {
					internal.Must(err)
				}
			}
		}
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateConnected {
			fmt.Println("Ctrl+C the remote client to stop the demo")
		} else if connectionState == webrtc.ICEConnectionStateFailed {
			if closeErr := wavFile.Close(); closeErr != nil {
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
	var s = &relay.NinjaSdp{}
	if err := utils.Decode(internal.MustReadStdin(), s); err != nil {
		panic(err)
	}
	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(*s.SDP)
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

	var a = &relay.NinjaSdp{
		Typ: relay.STAnswerToCaller,
		SID: s.SID,
		SDP: peerConnection.LocalDescription(),
	}

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(internal.Encode(a))

	// Block forever
	select {}
}
