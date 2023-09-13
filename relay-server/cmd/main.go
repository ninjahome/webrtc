package main

import (
	"github.com/ninjahome/webrtc/relay-server"
)

func main() {
	var rs = relay.NewServer()
	rs.StartSrv()
	select {}
}
