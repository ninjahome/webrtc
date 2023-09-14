package relay

import (
	"fmt"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/webrtc/v3"
	"io"
	"net/http"
	"sync"
)

const (
	MaxTunnelNum = 1 << 10
)

type Server struct {
	cacheLocker sync.RWMutex
	cache       map[string]*Tunnel
	tidErr      chan string
}

func NewServer() *Server {
	var rs = &Server{
		cache:  make(map[string]*Tunnel, MaxTunnelNum),
		tidErr: make(chan string, MaxTunnelNum),
	}
	return rs
}

func (rs *Server) StartSrv() {

	http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
		var s = &NinjaSdp{}
		body, _ := io.ReadAll(r.Body)

		if err := utils.Decode(string(body), s); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var a, err = rs.prepareSession(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var str, errCode = utils.Encode(a)
		if errCode != nil {
			http.Error(w, errCode.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(str))

		fmt.Println()
		fmt.Println(str)
		fmt.Println()
	})

	go func() {
		fmt.Println("relay server start success!!!")
		panic(http.ListenAndServe(":50000", nil))
	}()
}

func (rs *Server) prepareSession(sdp *NinjaSdp) (*NinjaSdp, error) {
	rs.cacheLocker.Lock()
	defer rs.cacheLocker.Unlock()

	switch sdp.Typ {

	case STCallerOffer:
		var sdpErr error
		var sdpA *webrtc.SessionDescription
		var tunnel, ok = rs.cache[sdp.SID]
		if ok {
			fmt.Println("old session exit:", sdp.SID)
			tunnel.Close()
		}

		tunnel, sdpA, sdpErr = NewTunnel(sdp, rs.tidErr)
		if sdpErr != nil {
			fmt.Println("create new tunnel err:", sdpErr)
			return nil, sdpErr
		}

		rs.cache[sdp.SID] = tunnel
		var answer = &NinjaSdp{
			Typ: STAnswerToCaller,
			SID: sdp.SID,
			SDP: sdpA,
		}

		fmt.Println("create new tunnel success:", answer.String())
		return answer, nil

	case STCalleeOffer:
		var tunnel, ok = rs.cache[sdp.SID]
		if !ok {
			fmt.Println("can't find caller's session:", sdp.SID)
			return nil, fmt.Errorf("no caller tunnel")
		}

		var sdpA, err = tunnel.UpdateTunnel(sdp)
		if err != nil {
			fmt.Println("update callee  sdp err:", err)
			rs.CloseTunnel(sdp.SID)
			return nil, err
		}
		var answer = &NinjaSdp{
			Typ: STAnswerToCallee,
			SID: sdp.SID,
			SDP: sdpA,
		}
		fmt.Println("update tunnel for callee success:", answer.String())
		return answer, nil
	}

	return nil, fmt.Errorf("unknown server sdp")
}

func (rs *Server) CloseTunnel(tid string) {
	fmt.Println("relay server is closing tunnel by id:=", tid)

	rs.cacheLocker.Lock()
	var t, ok = rs.cache[tid]
	if !ok {
		rs.cacheLocker.Unlock()
		return
	}
	delete(rs.cache, tid)
	rs.cacheLocker.Unlock()

	t.Close()
}

func (rs *Server) monitor() {
	for {
		select {
		case tid := <-rs.tidErr:
			rs.CloseTunnel(tid)

		}
	}
}
