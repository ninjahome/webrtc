package conn

import (
	"context"
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/ice/v2"
	"github.com/pion/stun"
	"sync"
	"time"
)

const (
	ICETimeOut = 40 * time.Second
	StunUrlStr = "stun:stun.l.google.com:19302"
)

const (
	AgentTypAudio AgentType = iota
	AgentTypeVideoOne
	AgentTypeVideoTwo
	AgentTypMax
)
const (
	CallTypeAudio CallType = iota + 1
	CallTypeVideo
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

type CallType byte
type AgentType byte

func (t AgentType) String() string {
	switch t {
	case AgentTypAudio:
		return "audio"
	case AgentTypeVideoOne:
		return "video1"
	case AgentTypeVideoTwo:
		return "video2"
	default:
		return "unknown ice agent type"
	}
}

type IceSdp [AgentTypMax]*IceConnParam

type IceConnParam struct {
	Candidates []string `json:"candidates"`
	Frag       string   `json:"frag"`
	Pwd        string   `json:"pwd"`
}

type OnConnected func(*ice.Conn)

type NinjaIceConn struct {
	callback ConnectCallBack

	agent    [AgentTypMax]*ice.Agent
	status   [AgentTypMax]ice.ConnectionState
	isCaller bool
	callTyp  CallType

	inVideoCache  chan []byte
	inAudioCache  chan []byte
	videoConnPair [2]*ice.Conn
}

func (nic *NinjaIceConn) createParam() (string, error) {

	var errCh []error
	var sdp IceSdp
	var wg sync.WaitGroup

	for i, a := range nic.agent {
		if a == nil {
			continue
		}
		var agent = a
		var idx = i
		wg.Add(1)

		go func() {
			defer wg.Done()

			var localFrag, localPwd, errUC = agent.GetLocalUserCredentials()
			if errUC != nil {
				errCh = append(errCh, errUC)
				return
			}
			var param = &IceConnParam{
				Frag: localFrag,
				Pwd:  localPwd,
			}

			var iceContext, iceCancel = context.WithCancel(context.TODO())
			var err = agent.OnCandidate(func(candidate ice.Candidate) {
				if candidate == nil {
					fmt.Println("======>>>candidate finding finished:", AgentType(idx).String())
					iceCancel()
					return
				}
				var c = candidate.Marshal()
				fmt.Println("======>>>candidate found:", AgentType(idx).String(), c)
				param.Candidates = append(param.Candidates, c)
			})
			if err != nil {
				errCh = append(errCh, err)
				return
			}
			err = agent.GatherCandidates()
			if err != nil {
				errCh = append(errCh, err)
				return
			}

			<-iceContext.Done()
			sdp[idx] = param
		}()
	}
	wg.Wait()

	var err = utils.FormatErr(errCh)
	if err != nil {
		return "", err
	}

	var pStr, errEn = utils.Encode(sdp)
	if errEn != nil {
		return "", errEn
	}
	return pStr, nil
}

func (nic *NinjaIceConn) IsConnected() bool {

	if nic.callTyp == CallTypeAudio {
		return nic.status[AgentTypAudio] == ice.ConnectionStateConnected
	}

	return nic.status[AgentTypAudio] == ice.ConnectionStateConnected &&
		nic.status[AgentTypeVideoOne] == ice.ConnectionStateConnected &&
		nic.status[AgentTypeVideoTwo] == ice.ConnectionStateConnected
}

func (nic *NinjaIceConn) Close() {
	if nic.inVideoCache == nil {
		return
	}

	for _, agent := range nic.agent {
		if agent == nil {
			continue
		}
		_ = agent.Close()
	}

	close(nic.inVideoCache)
	close(nic.inAudioCache)

	nic.inAudioCache = nil
	nic.inVideoCache = nil
}

func (nic *NinjaIceConn) SetRemoteDesc(offer string) error {
	var param IceSdp
	var err = utils.Decode(offer, &param)
	if err != nil {
		return err
	}
	fmt.Println("======>>>offer got:", param)
	var conns [AgentTypMax]*ice.Conn
	for i, param := range param {
		var agent = nic.agent[i]
		var aTyp = AgentType(i)

		if agent == nil {
			return fmt.Errorf("%s agent should not be nil", aTyp.String())
		}

		if len(param.Candidates) == 0 {
			return fmt.Errorf("no valid candidate for %s", aTyp.String())
		}

		for _, candidate := range param.Candidates {
			var can, err = ice.UnmarshalCandidate(candidate)
			if err != nil {
				return err
			}
			err = agent.AddRemoteCandidate(can)
			if err != nil {
				return err
			}
		}

		var conn *ice.Conn
		if nic.isCaller {
			conn, err = agent.Dial(context.TODO(), param.Frag, param.Pwd)
		} else {
			conn, err = agent.Accept(context.TODO(), param.Frag, param.Pwd)
		}
		if err != nil {
			return err
		}
		conns[i] = conn
		fmt.Println("======>>> connected for ", aTyp.String())
	}
	nic.OnConnected(conns)
	return nil
}

func (nic *NinjaIceConn) OnConnected(conn [AgentTypMax]*ice.Conn) {

	var qcConn = NewQueueConn(conn[AgentTypAudio])
	go nic.writingAudioToRemote(qcConn)
	go nic.readingAudioFromRemote(qcConn)
	go nic.writeAudioDataToApp()

	if nic.callTyp == CallTypeAudio {
		return
	}

	var v1Conn, v2Conn = NewQueueConn(conn[AgentTypeVideoOne]), NewQueueConn(conn[AgentTypeVideoTwo])

	if nic.isCaller {
		go nic.writeVideoToRemote(QCDataVideoOne, v1Conn)
		go nic.readVideoFromRemote(v2Conn)
	} else {
		go nic.writeVideoToRemote(QCDataVideoTwo, v2Conn)
		go nic.readVideoFromRemote(v1Conn)
	}
	go nic.writeVideoDataToApp()
}

func (nic *NinjaIceConn) writingAudioToRemote(conn *QueueConn) {

}

func (nic *NinjaIceConn) readingAudioFromRemote(conn *QueueConn) {

}

func (nic *NinjaIceConn) writeVideoToRemote(dType QCDataTye, conn *QueueConn) {
	var errCh = make(chan error, 2)

	go conn.readingLostSeq(errCh)
	go conn.WritingFrame(dType, nic.callback.RawCameraData, errCh)

	var err = <-errCh
	if err != nil {
		nic.callback.EndCall(err)
		conn.Close()
		nic.Close()
		return
	}
}

func (nic *NinjaIceConn) readVideoFromRemote(conn *QueueConn) {
	var err = conn.ReadFrameData(nic.inVideoCache)
	if err != nil {
		nic.callback.EndCall(err)
		conn.Close()
		nic.Close()
	}
	return
}
func (nic *NinjaIceConn) writeAudioDataToApp() {
}

func (nic *NinjaIceConn) writeVideoDataToApp() {
	for {
		var data, ok = <-nic.inVideoCache
		if !ok {
			nic.Close()
			nic.callback.EndCall(fmt.Errorf("data stream closed"))
			return
		}
		//fmt.Println("======>>>data from remote:", len(data), hex.EncodeToString(data))
		var _, err = nic.callback.GotVideoData(data)
		if err != nil {
			nic.callback.EndCall(err)
			nic.Close()
			return
		}
	}
}

func (nic *NinjaIceConn) createAgent(aTyp AgentType, back ConnectCallBack) (*ice.Agent, error) {
	var iceAgent, err = ice.NewAgent(iceConfig)
	if err != nil {
		return nil, err
	}

	err = iceAgent.OnConnectionStateChange(func(state ice.ConnectionState) {
		fmt.Printf("ICE Connection[%s] State has changed: %s\n",
			aTyp.String(), state.String())
		nic.status[aTyp] = state
		if state == ice.ConnectionStateFailed {
			back.EndCall(fmt.Errorf("ice connection failed"))
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return iceAgent, nil
}

func createBasicIceConn(callTyp CallType, back ConnectCallBack) (*NinjaIceConn, error) {

	var nic = &NinjaIceConn{
		callback:     back,
		callTyp:      callTyp,
		inVideoCache: make(chan []byte, MaxInBufferSize),
		inAudioCache: make(chan []byte, MaxInBufferSize),
	}

	var agent, err = nic.createAgent(AgentTypAudio, back)
	if err != nil {
		return nil, err
	}
	nic.agent[AgentTypAudio] = agent

	if callTyp == CallTypeAudio {
		return nic, nil
	}

	agent, err = nic.createAgent(AgentTypeVideoOne, back)
	if err != nil {
		return nil, err
	}
	nic.agent[AgentTypeVideoOne] = agent

	agent, err = nic.createAgent(AgentTypeVideoTwo, back)
	if err != nil {
		return nil, err
	}
	nic.agent[AgentTypeVideoTwo] = agent

	return nic, nil
}

func CreateCallerIceConn(callTyp CallType, back ConnectCallBack) (*NinjaIceConn, error) {
	var nic, err = createBasicIceConn(callTyp, back)
	if err != nil {
		return nil, err
	}
	nic.isCaller = true

	var offer, errOff = nic.createParam()
	if errOff != nil {
		return nil, errOff
	}
	back.OfferForCalleeCreated(offer)

	return nic, nil
}

func CreateCalleeIceConn(callTyp CallType, offer string, back ConnectCallBack) (*NinjaIceConn, error) {
	var nic, err = createBasicIceConn(callTyp, back)
	if err != nil {
		return nil, err
	}
	nic.isCaller = false

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
