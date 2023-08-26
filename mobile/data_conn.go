package webrtcLib

import (
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v3"
)

const (
	VideoDataChName = "ninja-data-video"
	AudioDataChName = "ninja-data-audio"
	MaxDataSize     = 1 << 24
)

type NinjaDataConn struct {
	status webrtc.PeerConnectionState
	*webrtc.PeerConnection
	callback ConnectCallBack
	inCache  chan []byte
}

func (ndc *NinjaDataConn) CreateCallerOffer() (string, error) {
	var offer, err = ndc.CreateOffer(nil)
	if err != nil {
		return "", err
	}
	offerGatheringComplete := webrtc.GatheringCompletePromise(ndc.PeerConnection)
	err = ndc.SetLocalDescription(offer)
	if err != nil {
		return "", err
	}
	<-offerGatheringComplete

	return utils.Encode(ndc.LocalDescription())
}

func (ndc *NinjaDataConn) OnVideoDataChOpen(channel *webrtc.DataChannel) {
	var raw, dErr = channel.Detach()
	if dErr != nil {
		ndc.callback.EndCall(dErr)
		return
	}
	go ndc.readingRemoteVideoData(raw)
	go ndc.writeRemoteDataToApp()
	go ndc.writeVideoDataToRemote(raw)
}

func (ndc *NinjaDataConn) Close() {

}

func (ndc *NinjaDataConn) IsConnected() bool {
	return ndc.status == webrtc.PeerConnectionStateConnected
}
func (ndc *NinjaDataConn) readingRemoteVideoData(raw datachannel.ReadWriteCloser) {

	var reader = &h264Conn{connReader: raw}

	for {
		var buf []byte
		var n, err = reader.Read(&buf)
		if err != nil || n == 0 {
			ndc.callback.EndCall(err)
			_ = raw.Close()
			return
		}
		fmt.Println("======>>>got from remote:", n) //, hex.EncodeToString(buf))
		ndc.inCache <- buf
	}
}

func (ndc *NinjaDataConn) writeRemoteDataToApp() {
	for {
		var data, ok = <-ndc.inCache
		if !ok {
			ndc.callback.EndCall(fmt.Errorf("no more remote data"))
			return
		}

		var _, err = ndc.callback.GotVideoData(data)
		if err != nil {
			ndc.callback.EndCall(err)
			return
		}
	}
}

func (ndc *NinjaDataConn) writeVideoDataToRemote(raw datachannel.ReadWriteCloser) {
	var writer = &h264Conn{connWriter: raw}
	for {
		var data, err = ndc.callback.RawCameraData()
		if err != nil {
			ndc.callback.EndCall(err)
			return
		}
		var n, errW = writer.Write(data)
		if errW != nil {
			ndc.callback.EndCall(errW)
			return
		}
		fmt.Println("======>>>write to peer :", len(data), n)
	}
}

func (ndc *NinjaDataConn) SetRemoteDesc(answer string) error {
	var sdp = webrtc.SessionDescription{}
	var err = utils.Decode(answer, &sdp)
	if err != nil {
		return err
	}

	return ndc.SetRemoteDescription(sdp)
}

func (ndc *NinjaDataConn) setLocalDesFromOffer(offerStr string) error {
	var offer = webrtc.SessionDescription{}
	if err := utils.Decode(offerStr, &offer); err != nil {
		return err
	}

	return ndc.SetRemoteDescription(offer)
}
func (ndc *NinjaDataConn) createAnswerForOffer() (string, error) {

	var answer, err = ndc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	gatherComplete := webrtc.GatheringCompletePromise(ndc.PeerConnection)

	if err = ndc.SetLocalDescription(answer); err != nil {
		return "", err
	}

	<-gatherComplete

	return utils.Encode(*ndc.LocalDescription())
}
func createBasicDataConn() (*NinjaDataConn, error) {
	var settingEngine = webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	var api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	var peerConnection, pcErr = api.NewPeerConnection(config)
	if pcErr != nil {
		return nil, pcErr
	}
	var ndc = &NinjaDataConn{
		status:  webrtc.PeerConnectionStateNew,
		inCache: make(chan []byte, MaxInBufferSize),
	}
	ndc.PeerConnection = peerConnection
	return ndc, nil
}

func CreateCallerDataConn(callback ConnectCallBack) (*NinjaDataConn, error) {

	var ndc, err = createBasicDataConn()
	if err != nil {
		return nil, err
	}
	ndc.callback = callback

	var videDataCh, DCErr = ndc.CreateDataChannel(VideoDataChName, nil)
	if DCErr != nil {
		return nil, err
	}
	videDataCh.OnOpen(func() {
		fmt.Printf("Data channel '%s'-'%d' open.\n", videDataCh.Label(), videDataCh.ID())
		ndc.OnVideoDataChOpen(videDataCh)
	})

	var offer, OfErr = ndc.CreateCallerOffer()
	if OfErr != nil {
		return nil, OfErr
	}
	ndc.callback.OfferForCalleeCreated(offer)

	ndc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", state.String())
		ndc.status = state
		if state == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			ndc.callback.EndCall(fmt.Errorf("connection failed"))
		}
	})

	return ndc, nil
}

func CreateCalleeDataConn(offerStr string, callback ConnectCallBack) (*NinjaDataConn, error) {

	var ndc, err = createBasicDataConn()
	if err != nil {
		return nil, err
	}
	ndc.callback = callback

	err = ndc.SetRemoteDesc(offerStr)
	if err != nil {
		return nil, err
	}

	ndc.OnDataChannel(func(channel *webrtc.DataChannel) {
		fmt.Printf("======>>>New DataChannel %s %d\n", channel.Label(), channel.ID())
		if channel.Label() == VideoDataChName {
			channel.OnOpen(func() {
				fmt.Printf("======>>>Data channel '%s'-'%d' open.\n", channel.Label(), channel.ID())
				ndc.OnVideoDataChOpen(channel)
			})
		}

	})
	ndc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())
		ndc.status = s
		if s == webrtc.PeerConnectionStateFailed {
			fmt.Println("Peer Connection has gone to failed exiting")
			callback.EndCall(fmt.Errorf("connection failed"))
		}
	})
	var answer, errAnswer = ndc.createAnswerForOffer()
	if errAnswer != nil {
		return nil, errAnswer
	}
	ndc.callback.AnswerForCallerCreated(answer)
	return ndc, nil
}
