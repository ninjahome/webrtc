package webrtcLib

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/ice/v2"
	"github.com/pion/stun"
	"time"
)

const (
	ICETimeOut = 40 * time.Second
	StunUrlStr = "stun:stun.l.google.com:19302"
)

type IceConnParam struct {
	Candidates []string `json:"candidates"`
	Frag       string   `json:"frag"`
	Pwd        string   `json:"pwd"`
}
type OnConnected func(*ice.Conn)

type NinjaIceConn struct {
	callback    ConnectCallBack
	agent       *ice.Agent
	status      ice.ConnectionState
	iceContext  context.Context
	iceCancel   context.CancelFunc
	isOffer     bool
	onConnected OnConnected
	inCache     chan []byte
}

func (nic *NinjaIceConn) createParam() (string, error) {
	var localFrag, localPwd, errUC = nic.agent.GetLocalUserCredentials()
	if errUC != nil {
		return "", errUC
	}
	var param = &IceConnParam{
		Frag: localFrag,
		Pwd:  localPwd,
	}
	var err = nic.agent.OnCandidate(func(candidate ice.Candidate) {
		if candidate == nil {
			fmt.Println("======>>>candidate finding finished")
			nic.iceCancel()
			return
		}
		var c = candidate.Marshal()
		fmt.Println("======>>>candidate found:", c)
		param.Candidates = append(param.Candidates, c)
	})
	if err != nil {
		return "", err
	}
	err = nic.agent.GatherCandidates()
	if err != nil {
		return "", err
	}

	<-nic.iceContext.Done()
	var pStr, errEn = utils.Encode(param)
	if errEn != nil {
		return "", errEn
	}
	return pStr, nil
}
func (nic *NinjaIceConn) IsConnected() bool {
	return nic.status == ice.ConnectionStateConnected
}

func (nic *NinjaIceConn) Close() {
	nic.iceCancel()
}

func (nic *NinjaIceConn) SetRemoteDesc(offer string) error {
	var param = &IceConnParam{}
	var err = utils.Decode(offer, param)
	if err != nil {
		return err
	}
	//fmt.Println("======>>>offer got:", param)
	if len(param.Candidates) == 0 {
		return fmt.Errorf("no valid candidate")
	}

	for _, candidate := range param.Candidates {
		var can, err = ice.UnmarshalCandidate(candidate)
		if err != nil {
			return err
		}
		err = nic.agent.AddRemoteCandidate(can)
		if err != nil {
			return err
		}
	}

	var conn *ice.Conn
	if nic.isOffer {
		conn, err = nic.agent.Dial(context.TODO(), param.Frag, param.Pwd)
	} else {
		conn, err = nic.agent.Accept(context.TODO(), param.Frag, param.Pwd)
	}
	if err != nil {
		return err
	}
	nic.onConnected(conn)
	return nil
}

func (nic *NinjaIceConn) iceConnectionOn(conn *ice.Conn) {
	go nic.writeVideoToRemote(conn)
	go nic.readVideoFromRemote(conn)
	go nic.writeDataToApp()
}

func (nic *NinjaIceConn) writeVideoToRemote(conn *ice.Conn) {
	var writer = NewQueueConn(conn, conn) //{connReader: conn}
	for {
		var data, err = nic.callback.RawCameraData()
		if err != nil {
			nic.callback.EndCall(err)
			_ = conn.Close()
			return
		}
		_, err = writer.writeFrameData(data)
		if err != nil {
			nic.callback.EndCall(err)
			_ = conn.Close()
			return
		}
	}
}

func (nic *NinjaIceConn) readVideoFromRemote(conn *ice.Conn) {
	var reader = NewQueueConn(conn, conn) //{connReader: conn}
	var err = reader.ReadFrameData(nic.inCache)
	if err != nil {
		nic.callback.EndCall(fmt.Errorf("read video finished"))
		_ = conn.Close()
	}
	return
}

func (nic *NinjaIceConn) writeDataToApp() {
	for {
		var data, ok = <-nic.inCache
		if !ok {
			nic.callback.EndCall(fmt.Errorf("no more remote data"))
			return
		}

		var _, err = nic.callback.GotVideoData(data)
		if err != nil {
			nic.callback.EndCall(err)
			return
		}
	}
}

func createBasicIceConn(back ConnectCallBack) (*NinjaIceConn, error) {
	var timeOut = ICETimeOut
	var stunUrl, _ = stun.ParseURI(StunUrlStr)
	var iceConfig = &ice.AgentConfig{
		NetworkTypes:  []ice.NetworkType{ice.NetworkTypeUDP4},
		Urls:          []*stun.URI{stunUrl},
		FailedTimeout: &timeOut,
	}
	var iceCtx, iceCancel = context.WithCancel(context.TODO())
	var nic = &NinjaIceConn{
		status:     ice.ConnectionStateNew,
		callback:   back,
		iceContext: iceCtx,
		iceCancel:  iceCancel,
		inCache:    make(chan []byte, MaxInBufferSize),
	}

	nic.onConnected = nic.iceConnectionOn

	var iceAgent, err = ice.NewAgent(iceConfig)
	if err != nil {
		return nil, err
	}
	nic.agent = iceAgent

	err = iceAgent.OnConnectionStateChange(func(state ice.ConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", state.String())
		nic.status = state
		if state == ice.ConnectionStateFailed {
			back.EndCall(fmt.Errorf("ice connection failed"))
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return nic, nil
}

func CreateCallerIceConn(back ConnectCallBack) (*NinjaIceConn, error) {
	var nic, err = createBasicIceConn(back)
	if err != nil {
		return nil, err
	}
	nic.isOffer = true

	var offer, errOff = nic.createParam()
	if errOff != nil {
		return nil, errOff
	}
	back.OfferForCalleeCreated(offer)

	return nic, nil
}

func CreateCalleeIceConn(offer string, back ConnectCallBack) (*NinjaIceConn, error) {
	var nic, err = createBasicIceConn(back)
	if err != nil {
		return nil, err
	}
	nic.isOffer = false

	var answer, errOff = nic.createParam()
	if errOff != nil {
		return nil, errOff
	}

	back.AnswerForCallerCreated(answer)

	err = nic.SetRemoteDesc(offer)
	if err != nil {
		return nil, err
	}
	return nic, nil
}
