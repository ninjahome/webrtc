package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	"github.com/pion/datachannel"
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
)

func WriteLoop(peer datachannel.ReadWriteCloser, videoReader mediadevices.RTPReadCloser) {
	var writer = h264writer.NewWith(peer)
	for {
		packs, release, err := videoReader.Read()
		if err != nil {
			panic(err)
		}
		for _, packet := range packs {
			err = writer.WriteRTP(packet)
			if err != nil {
				peer.Close()
				videoReader.Close()
				return
			}
			release()
		}
	}
}

func ReadLoop(peer datachannel.ReadWriteCloser, writer io.WriteCloser) {
	for {
		var lenBuf = make([]byte, 4)
		n, err := io.ReadFull(peer, lenBuf)
		if err != nil {
			panic(err)
		}
		var dataLen = binary.BigEndian.Uint32(lenBuf)
		fmt.Println("======>>>data len", dataLen)
		if dataLen > 1<<24 {
			panic("too big data")
		}
		var buffer = make([]byte, dataLen)
		n, err = peer.Read(buffer)
		if err != nil {
			fmt.Println("Datachannel closed; Exit the readloop:", err)
			peer.Close()
			writer.Close()
			return
		}
		fmt.Println("======>>>got from peer", hex.EncodeToString(buffer[:n]))

		writer.Write(buffer[:n])
	}
}

func main() {
	offerAddr := flag.String("offer-address", ":50000", "Address that the Offer HTTP server is hosted on.")
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	x264Params, errX264 := x264.NewParams()
	internal.Must(errX264)
	x264Params.BitRate = 1_000_1000

	opusParams, errOpus := opus.NewParams()
	internal.Must(errOpus)
	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	s := webrtc.SettingEngine{}
	s.DetachDataChannels()

	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	var peerConnection, err = api.NewPeerConnection(config)
	internal.Must(err)
	dc, err := peerConnection.CreateDataChannel("ninja-data-video", nil)
	internal.Must(err)

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

	videoTrack := mediaStream.GetVideoTracks()[0].(*mediadevices.VideoTrack)
	defer videoTrack.Close()
	_, err = videoTrack.NewRTPReader(x264Params.RTPCodec().MimeType, rand.Uint32(), 1000)

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
	var ivfFile, ivfErr = os.Create("offer.h264")
	internal.Must(ivfErr)
	dc.OnOpen(func() {
		fmt.Printf("Data channel '%s'-'%d' open.\n", dc.Label(), dc.ID())

		// Detach the data channel
		raw, dErr := dc.Detach()
		if dErr != nil {
			panic(dErr)
		}

		// Handle reading from the data channel
		go ReadLoop(raw, ivfFile)

		// Handle writing to the data channel
		//go WriteLoop(raw, reader)
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
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}
	})

	select {}
}
