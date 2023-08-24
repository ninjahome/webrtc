package internal

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"io"
	"os"
	"strings"
)

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

const (
	MTU = 1000
)

func Encode(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}
func Decode(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
}
func MustReadStdin() string {
	r := bufio.NewReader(os.Stdin)

	var in string
	for {
		var err error
		in, err = r.ReadString('\n')
		if err != io.EOF {
			if err != nil {
				panic(err)
			}
		}
		in = strings.TrimSpace(in)
		if len(in) > 0 {
			break
		}
	}

	fmt.Println("")

	return in
}

func SaveToDisk(i media.Writer, track *webrtc.TrackRemote) {
	defer func() {
		if err := i.Close(); err != nil {
			panic(err)
		}
	}()

	for {

		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			panic(err)
		}
		fmt.Println("------>>>writing rtpPacket:", rtpPacket.String())
		if err := i.WriteRTP(rtpPacket); err != nil {
			fmt.Println(err, "\n", rtpPacket.String())
		}
	}
}
