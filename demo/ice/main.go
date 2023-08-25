package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/ninjahome/webrtc/demo/internal"
	webrtcLib "github.com/ninjahome/webrtc/mobile"
	"github.com/pion/ice/v2"
	"github.com/pion/randutil"
	"github.com/pion/stun"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	ICETimeOut = 40 * time.Second
)

var (
	isControlling bool
	iceAgent      *ice.Agent
)

func main() {
	offerAddr := flag.String("offer-address", ":50000", "Address that the Offer HTTP server is hosted on.")
	flag.BoolVar(&isControlling, "offer", false, "is ICE Agent controlling")
	flag.Parse()

	if isControlling {
		fmt.Println("Local Agent is controlling")
	} else {
		fmt.Println("Local Agent is controlled")
	}

	u, err := stun.ParseURI("stun:stun.l.google.com:19302")
	if err != nil {
		panic(err)
	}
	var timeOut = ICETimeOut
	sig := &webrtcLib.IceConnParam{}
	iceAgent, err = ice.NewAgent(&ice.AgentConfig{
		NetworkTypes:  []ice.NetworkType{ice.NetworkTypeUDP4, ice.NetworkTypeUDP6},
		Urls:          []*stun.URI{u},
		FailedTimeout: &timeOut,
	})
	if err != nil {
		panic(err)
	}
	iceCtx, iceCancel := context.WithCancel(context.TODO())
	if err = iceAgent.OnCandidate(func(c ice.Candidate) {
		if c == nil {
			iceCancel()
			fmt.Println("=============>>>")
			return
		}
		fmt.Println("OnCandidate success", c.Marshal())

		sig.Candidates = append(sig.Candidates, c.Marshal())
	}); err != nil {
		panic(err)
	}
	if err = iceAgent.OnConnectionStateChange(func(c ice.ConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", c.String())
	}); err != nil {
		panic(err)
	}

	// Get the local auth details and send to remote peer
	localUfrag, localPwd, err := iceAgent.GetLocalUserCredentials()
	if err != nil {
		panic(err)
	}

	sig.Frag = localUfrag
	sig.Pwd = localPwd

	if err = iceAgent.GatherCandidates(); err != nil {
		panic(err)
	}

	<-iceCtx.Done()

	fmt.Println(sig)
	fmt.Println(internal.Encode(sig))

	remoteSig := make(chan *webrtcLib.IceConnParam, 1)
	if isControlling {
		http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
			s := &webrtcLib.IceConnParam{}
			body, _ := io.ReadAll(r.Body)
			internal.Decode(string(body), s)
			remoteSig <- s
		})
		go func() { panic(http.ListenAndServe(*offerAddr, nil)) }()
	} else {
		s := &webrtcLib.IceConnParam{}
		internal.Decode(internal.MustReadStdin(), &s)
		remoteSig <- s
	}

	remote := <-remoteSig
	fmt.Println(remote)

	if len(remote.Candidates) == 0 {
		panic("no valid candidates")
	}
	for _, candidate := range remote.Candidates {
		c, err := ice.UnmarshalCandidate(candidate)
		if err != nil {
			panic(err)
		}
		if err := iceAgent.AddRemoteCandidate(c); err != nil {
			panic(err)
		}
	}

	var conn *ice.Conn
	if isControlling {
		conn, err = iceAgent.Dial(context.TODO(), remote.Frag, remote.Pwd)
	} else {
		conn, err = iceAgent.Accept(context.TODO(), remote.Frag, remote.Pwd)
	}
	if err != nil {
		panic(err)
	}
	// Send messages in a loop to the remote peer
	go func() {
		for {
			time.Sleep(time.Second * 3)

			val, err := randutil.GenerateCryptoRandomString(15, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
			if err != nil {
				panic(err)
			}
			if _, err = conn.Write([]byte(val)); err != nil {
				panic(err)
			}

			fmt.Printf("Sent: '%s'\n", val)
		}
	}()

	// Receive messages in a loop from the remote peer
	var file, errF = os.Create("offer.h264")
	if errF != nil {
		panic(errF)
	}
	var lenBuf = make([]byte, 4)
	var MaxDataSize = 1 << 24
	for {
		var n, err = io.ReadFull(conn, lenBuf)
		if err != nil || n != 4 {
			panic(err)
		}
		var dataLen = int(binary.BigEndian.Uint32(lenBuf))
		fmt.Println("======>>>data len", dataLen)
		if dataLen > MaxDataSize {
			panic("too big data")
		}

		var buffer = make([]byte, dataLen)
		n, err = io.ReadFull(conn, buffer)
		if err != nil || n != dataLen {
			panic(err)
		}

		fmt.Println("Received:", dataLen)
		n, err = file.Write(buffer)
		if err != nil {
			panic(err)
		}
	}
}
