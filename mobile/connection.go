package webrtcLib

type NinjaConn interface {
	IsConnected() bool
	Close()
	SetRemoteDesc(string) error
}
