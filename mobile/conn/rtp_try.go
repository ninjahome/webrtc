package conn

import (
	"fmt"
	"github.com/ninjahome/webrtc/relay-server"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/webrtc/v3"
)

func CreateCalleeRtpConn(hasVideo bool, offerStr string, callback ConnectCallBack) (*NinjaRtpConn, error) {
	fmt.Println("======>>>start to create answering conn")
	var nc, err = createBasicConn(hasVideo, callback)
	if err != nil {
		return nil, err
	}

	if err := nc.SetRemoteDesc(offerStr); err != nil {
		return nil, err
	}

	nc.conn.OnTrack(nc.OnTrack)
	nc.conn.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		nc.status = s
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		if s == webrtc.PeerConnectionStateConnected {
			nc.relayStart()
			callback.CallStart()
		}
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			nc.callback.EndCallByInnerErr(fmt.Errorf("connection status:%s", s))
		}
	})

	var answer, errA = nc.createAnswerForCaller()
	if errA != nil {
		return nil, errA
	}
	nc.callback.AnswerForCallerCreated(answer)

	return nc, nil
}

func (nc *NinjaRtpConn) createAnswerForCaller() (string, error) {
	fmt.Println("======>>>creating answer for caller")
	var answer, errA = nc.conn.CreateAnswer(nil)
	if errA != nil {

		return "", errA
	}

	gatherComplete := webrtc.GatheringCompletePromise(nc.conn)

	if err := nc.conn.SetLocalDescription(answer); err != nil {
		return "", err
	}

	<-gatherComplete

	var sdp = &relay.NinjaSdp{
		Typ: relay.STAnswerToCaller,
		SID: "from-to-ninja-ids", //TODO:: refactor this later
		SDP: nc.conn.LocalDescription(),
	}

	var answerStr, err = utils.Encode(sdp)
	if err != nil {
		return "", err
	}
	fmt.Println(sdp.SDP)
	return answerStr, nil
}
