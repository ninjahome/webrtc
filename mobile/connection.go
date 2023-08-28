package webrtcLib

const (
	IceUdpMtu = 1<<13 - NinHeaderLen
)

type NinjaConn interface {
	IsConnected() bool
	Close()
	SetRemoteDesc(string) error
}
