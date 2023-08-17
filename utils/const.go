package utils

import "github.com/pion/webrtc/v3"

const (
	StunAgent = "stun:stun.l.google.com:19302"
)

var (
	DefaultICECfg = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

var (
	Version   string
	Commit    string
	BuildTime string
)
