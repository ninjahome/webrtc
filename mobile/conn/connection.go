package conn

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/pion/webrtc/v3"
)

const (
	IceUdpMtu         = 1<<13 - NinHeaderLen
	H264TypMask       = 0x1f
	MaxConnBufferSize = 1 << 22
	MaxInBufferSize   = 1 << 10
	VideoAvcLen       = 4
)

var (
	config = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun1.l.google.com:19302",
					"stun:stun2.l.google.com:19302",
				},
			},
		},
	}
	VideoAvcStart = []byte{0x00, 0x00, 0x00, 0x01}
)

type NinjaConn interface {
	IsConnected() bool
	Close()
	SetRemoteDesc(string) error
}

func H254Write(p []byte, callback func(typ int, h264data []byte)) (n int, err error) {
	if len(p) < 5 {
		fmt.Println("======>>>invalid rtp packets:", p)
		return 0, nil
	}

	var startIdx = bytes.Index(p, VideoAvcStart)
	if startIdx != 0 {
		return 0, fmt.Errorf("invalid h64 stream data\n%v", hex.EncodeToString(p))
	}

	var typ = int(p[VideoAvcLen] & H264TypMask)
	var origLen = len(p)
	p = p[VideoAvcLen:]
	if typ == 7 {
		startIdx = bytes.Index(p, VideoAvcStart)
		if startIdx < 0 {
			callback(typ, p)
			return origLen, nil
		}
		callback(typ, p[:startIdx])

		p = p[startIdx+VideoAvcLen:]
		var nextTyp = int(p[0] & H264TypMask)
		if nextTyp != 8 {
			return 0, fmt.Errorf("error pps frame")
		}
		callback(nextTyp, p)
		return origLen, nil
	}

	if typ > 0 {
		callback(typ, p)
		return origLen, nil
	}

	return 0, fmt.Errorf("invalid h64 stream data\n%v", p)
}
