package main

import (
	"flag"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	webrtcLib "github.com/ninjahome/webrtc/mobile/conn"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	ICETimeOut = 40 * time.Second
)

var (
	isCallee bool
)

type fakeApp struct {
	file *os.File
}

func (f fakeApp) GotVideoData(buf []byte) (n int, err error) {
	return f.file.Write(buf)
}

func (f fakeApp) RawCameraData() ([]byte, error) {
	select {}
}

func (f fakeApp) RawMicroData() ([]byte, error) {
	select {}
}

func (f fakeApp) AnswerForCallerCreated(s string) {
	fmt.Println(s)
}

func (f fakeApp) OfferForCalleeCreated(s string) {
	fmt.Println(s)
}

func (f fakeApp) EndCall(err error) {
	fmt.Println(err)
	os.Exit(0)
}

func main() {

	offerAddr := flag.String("offer-address", ":50000", "Address that the Offer HTTP server is hosted on.")
	flag.BoolVar(&isCallee, "a", false, "is ICE Agent controlling")
	flag.Parse()
	var file, errW = os.Create("offer.h264")
	if errW != nil {
		panic(errW)
	}
	var fa = &fakeApp{file}
	var peerConnection *webrtcLib.NinjaIceConn
	var err error

	if isCallee {
		var offer = internal.MustReadStdin()
		peerConnection, err = webrtcLib.CreateCalleeIceConn(webrtcLib.CallTypeVideo, offer, fa)

	} else {

		peerConnection, err = webrtcLib.CreateCallerIceConn(webrtcLib.CallTypeVideo, fa)

		http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			err = peerConnection.SetRemoteDesc(string(body))
			if err != nil {
				panic(err)
			}
		})
		go func() { panic(http.ListenAndServe(*offerAddr, nil)) }()
	}

	if err != nil {
		panic(err)
	}

	select {}
}
