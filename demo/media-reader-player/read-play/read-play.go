package main

import (
	"flag"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
	"io"
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

	var mediaRegVideoErr = m.RegisterCodec(codec.NewRTPVP8Codec(90000).RTPCodecParameters, webrtc.RTPCodecTypeVideo)
	internal.Must(mediaRegVideoErr)
	var mediaRegAudioErr = m.RegisterCodec(codec.NewRTPOpusCodec(48000).RTPCodecParameters, webrtc.RTPCodecTypeAudio)
	internal.Must(mediaRegAudioErr)

	i := &interceptor.Registry{}

	intervalPliFactory, ipErr := intervalpli.NewReceiverInterceptor()
	internal.Must(ipErr)
	i.Add(intervalPliFactory)
	var rgeErr = webrtc.RegisterDefaultInterceptors(m, i)
	internal.Must(rgeErr)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	var peerConnection, err = api.NewPeerConnection(config)
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

	var oggFile, oggErr = oggwriter.New("output.ogg", 48000, 2)
	internal.Must(oggErr)
	var ivfFile, ivfErr = ivfwriter.New("output_read_play.ivf") //, ivfwriter.WithCodec(webrtc.MimeTypeAV1))
	internal.Must(ivfErr)

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := track.Codec()
		fmt.Println("------>>>codec:", codec)
		if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
			internal.SaveToDisk(oggFile, track)
		} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeH264) {
			internal.SaveToDisk(ivfFile, track)
		} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeVP8) {
			internal.SaveToDisk(ivfFile, track)
		} else if strings.EqualFold(track.Codec().MimeType, webrtc.MimeTypeAV1) {
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

	select {}
}
