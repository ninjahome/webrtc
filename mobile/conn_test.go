package webrtcLib

import (
	"fmt"
	"testing"
)

type testConnCallback struct {
}

func (t testConnCallback) NewVideoData(typ int, h264data []byte) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) P2pConnected() {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) AnswerCreated(s string) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) OfferCreated(s string) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) GotVideoData(p []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) RawCameraData() ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) RawMicroData() ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) AnswerForCallerCreated(s string) {
	//TODO implement me
	panic("implement me")
}

func (t testConnCallback) OfferForCalleeCreated(s string) {
	fmt.Println(s)
}

func (t testConnCallback) EndCall(err error) {
	//TODO implement me
	panic("implement me")
}

func TestCreateConnectionAsCaller(t *testing.T) {
	var cb = &testConnCallback{}
	var conn, err = CreateConnectionAsCaller(cb)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}

func TestStartVideo(t *testing.T) {
	var cb = &testConnCallback{}
	var err = StartVideo(cb)
	if err != nil {
		t.Fatal(err)
	}
}
