package webrtcLib

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/ice/v2"
	"github.com/pion/stun"
	"io"
	"time"
)

const (
	ICETimeOut = 40 * time.Second
	StunUrlStr = "stun:stun.l.google.com:19302"
	LenBufSize = 4
)

var (
	timeOut    = ICETimeOut
	stunUrl, _ = stun.ParseURI(StunUrlStr)
	iceConfig  = &ice.AgentConfig{
		NetworkTypes:  []ice.NetworkType{ice.NetworkTypeUDP4, ice.NetworkTypeUDP6},
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
	var lenBuf = make([]byte, LenBufSize)
	for {
		var writeN = 0
		var errW error
		var data, err = nic.callback.RawCameraData()
		var dataLen = len(data)

		fmt.Println("======>>>got from camera:", dataLen, hex.EncodeToString(data))
		if err != nil {
			nic.callback.EndCall(err)
			return
		}

		binary.BigEndian.PutUint32(lenBuf, uint32(dataLen))
		writeN, errW = conn.Write(lenBuf)
		if writeN != LenBufSize || errW != nil {
			nic.callback.EndCall(fmt.Errorf("write err:%s write n:%d", errW, writeN))
			return
		}

		for startIdx := 0; startIdx < dataLen; startIdx = startIdx + IceUdpMtu {
			if startIdx+IceUdpMtu > dataLen {
				writeN, errW = conn.Write(data[startIdx:dataLen])
			} else {
				writeN, errW = conn.Write(data[startIdx : startIdx+IceUdpMtu])
			}
			if errW != nil || writeN == 0 {
				nic.callback.EndCall(fmt.Errorf("write err:%s write n:%d", errW, writeN))
				return
			}
			fmt.Println("======>>>write to peer :", startIdx, writeN)
		}
	}
}

func (nic *NinjaIceConn) readVideoFromRemote(conn *ice.Conn) {
	var lenBuf = make([]byte, LenBufSize)
	for {
		var n, err = io.ReadFull(conn, lenBuf)
		if err != nil {
			nic.callback.EndCall(err)
			_ = conn.Close()
			return
		}
		var dataLen = int(binary.BigEndian.Uint32(lenBuf))
		fmt.Println("======>>>read remote video dataLen:", dataLen)
		if dataLen > MaxDataSize {
			nic.callback.EndCall(fmt.Errorf("too big data size:%d", dataLen))
			_ = conn.Close()
			return
		}
		var buf = make([]byte, dataLen)
		n, err = io.ReadFull(conn, buf)
		if n != dataLen || err != nil {
			nic.callback.EndCall(fmt.Errorf("======>>>read video data failed %d-%s", n, err))
			_ = conn.Close()
			return
		}
		fmt.Println("======>>>got from remote:", dataLen, hex.EncodeToString(buf))

		nic.inCache <- buf
	}
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
