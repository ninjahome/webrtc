package relay

import (
	"fmt"
	"github.com/ninjahome/webrtc/utils"
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
}

func NewServer() *Server {
	var rs = &Server{
		cache: make(map[string]*Tunnel, MaxTunnelNum),
	}
	return rs
}

func (rs *Server) StartSrv() {

	http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
		var s = &NinjaSdp{}
		body, _ := io.ReadAll(r.Body)
		if err := utils.Decode(string(body), s); err != nil {
			panic(err)
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
	})

	go func() { panic(http.ListenAndServe(":50000", nil)) }()
}

func (rs *Server) prepareSession(sdp *NinjaSdp) (*NinjaSdp, error) {
	rs.cacheLocker.Lock()
	defer rs.cacheLocker.Unlock()

	switch sdp.Typ {

	case STCallerOffer:
		var sdpErr error
		var tunnel, ok = rs.cache[sdp.SID]
		if ok {
			tunnel.Close()
		}

		tunnel, sdpErr = NewTunnel(sdp, rs.CloseTunnel)
		if sdpErr != nil {
			return nil, sdpErr
		}

		rs.cache[sdp.SID] = tunnel
		var sdpA = tunnel.CallerAnswerSdp()
		var answer = &NinjaSdp{
			Typ: STAnswerToCaller,
			SID: sdp.SID,
			SDP: sdpA,
		}

		return answer, nil

	case STCalleeOffer:
		var tunnel, ok = rs.cache[sdp.SID]
		if !ok {
			return nil, fmt.Errorf("no caller tunnel")
		}

		var sdpA, err = tunnel.UpdateCalleeSdp(sdp)
		if err != nil {
			rs.CloseTunnel(sdp.SID)
			return nil, err
		}
		var answer = &NinjaSdp{
			Typ: STAnswerToCallee,
			SID: sdp.SID,
			SDP: sdpA,
		}

		return answer, nil
	}

	return nil, fmt.Errorf("unknown server sdp")
}

func (rs *Server) CloseTunnel(tid string) {
	rs.cacheLocker.Lock()
	defer rs.cacheLocker.Unlock()
	var t, ok = rs.cache[tid]
	if !ok {
		return
	}
	t.Close()
	delete(rs.cache, tid)
}
