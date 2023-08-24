package webrtcLib

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v3"
	"os"
)

func createP2pDataConn(offerStr string, callback ConnectCallBack) (*webrtc.PeerConnection, error) {
	s := webrtc.SettingEngine{}
	s.DetachDataChannels()
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))
	var peerConnection, pcErr = api.NewPeerConnection(config)
	if pcErr != nil {
		return nil, pcErr
	}

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

	// Register data channel creation handling
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

		// Register channel opening handling
		d.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open.\n", d.Label(), d.ID())

			// Detach the data channel
			raw, dErr := d.Detach()
			if dErr != nil {
				panic(dErr)
			}

			// Handle reading from the data channel
			go ReadLoop(raw, callback)

			// Handle writing to the data channel
			go WriteLoop(raw, callback)
		})
	})

	offer := webrtc.SessionDescription{}
	utils.Decode(offerStr, &offer)

	pcErr = peerConnection.SetRemoteDescription(offer)
	if pcErr != nil {
		return nil, pcErr
	}

	var answer, err = peerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	<-gatherComplete

	fmt.Println(utils.Encode(*peerConnection.LocalDescription()))

	return peerConnection, nil
}

func WriteLoop(raw datachannel.ReadWriteCloser, callback ConnectCallBack) {
	for {
		var data, err = callback.RawCameraData()
		if err != nil {
			callback.EndCall()
			raw.Close()
			return
		}
		fmt.Println("======>>>write to peer", hex.EncodeToString(data))
		var lenBuf = make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
		var _, err1 = raw.Write(lenBuf)
		var _, err2 = raw.Write(data)
		if err1 != nil || err2 != nil {
			callback.EndCall()
			raw.Close()
			return
		}
	}
}

func ReadLoop(raw datachannel.ReadWriteCloser, callback ConnectCallBack) {

}
