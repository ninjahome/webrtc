package webrtcLib

import (
	"context"
	"encoding/binary"
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

var (
	timeOut    = ICETimeOut
	stunUrl, _ = stun.ParseURI("stun:stun.l.google.com:19302")
	iceConfig  = &ice.AgentConfig{
		NetworkTypes:  []ice.NetworkType{ice.NetworkTypeUDP4},
		Urls:          []*stun.URI{stunUrl},
		FailedTimeout: &timeOut,
	}
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
}

func (nic *NinjaIceConn) writeVideoToRemote(conn *ice.Conn) {
	var lenBuf = make([]byte, 4)
	for {
		var writeN = 0
		var errW error
		var data, err = nic.callback.RawCameraData()
		var dateLen = len(data)

		fmt.Println("======>>>got from camera:", dateLen)
		if err != nil {
			nic.callback.EndCall(err)
			return
		}

		binary.BigEndian.PutUint32(lenBuf, uint32(dateLen))
		writeN, errW = conn.Write(lenBuf)
		if writeN != 4 || errW != nil {
			nic.callback.EndCall(fmt.Errorf("write err:%s write n:%d", errW, writeN))
			return
		}

		for startIdx := 0; startIdx < dateLen; startIdx = startIdx + IceUdpMtu {
			if startIdx+IceUdpMtu > dateLen {
				writeN, errW = conn.Write(data[startIdx:dateLen])
			} else {
				writeN, errW = conn.Write(data[startIdx : startIdx+IceUdpMtu])
			}
			if errW != nil || writeN == 0 {
				nic.callback.EndCall(fmt.Errorf("write err:%s write n:%d", errW, writeN))
				return
			}
			//fmt.Println("======>>>write to peer :", writeN, startIdx, IceUdpMtu)
		}
	}
}

func (nic *NinjaIceConn) readVideoFromRemote(conn *ice.Conn) {
	var buf = make([]byte, MaxConnBufferSize)
	for {
		var n, err = conn.Read(buf)
		if err != nil {
			nic.callback.EndCall(err)
			return
		}
		fmt.Println("======>>>", n)
		_, err = nic.callback.GotVideoData(buf[:n])
		if err != nil {
			nic.callback.EndCall(err)
			return
		}
	}
}

func createBasicIceConn(back ConnectCallBack) (*NinjaIceConn, error) {
	var iceCtx, iceCancel = context.WithCancel(context.TODO())
	var nic = &NinjaIceConn{
		status:     ice.ConnectionStateNew,
		callback:   back,
		iceContext: iceCtx,
		iceCancel:  iceCancel,
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
			iceCancel()
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
