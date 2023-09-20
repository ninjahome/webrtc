package utils

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pion/randutil"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"io"
)

const (
	runesAlpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	compress   = false
)

// Use global random generator to properly seed by crypto grade random.
var globalMathRandomGenerator = randutil.NewMathRandomGenerator() // nolint:gochecknoglobals

// MathRandAlpha generates a mathmatical random alphabet sequence of the requested length.
func MathRandAlpha(n int) string {
	return globalMathRandomGenerator.GenerateString(n, runesAlpha)
}

// RandUint32 generates a mathmatical random uint32.
func RandUint32() uint32 {
	return globalMathRandomGenerator.Uint32()
}

func Decode(in string, obj interface{}) error {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return err
	}

	if compress {
		b, err = unzip(b)
		if err != nil {
			return err
		}
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		return err
	}
	return nil
}
func zip(in []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	_, err := gz.Write(in)
	if err != nil {
		return nil, err
	}
	err = gz.Flush()
	if err != nil {
		return nil, err
	}
	err = gz.Close()
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func unzip(in []byte) ([]byte, error) {
	var b bytes.Buffer
	_, err := b.Write(in)
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(&b)
	if err != nil {
		return nil, err
	}
	res, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func Encode(obj interface{}) (string, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}

	if compress {
		b, err = zip(b)
		if err != nil {
			return "", err
		}
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

func FormatErr(errs []error) error {
	if errs == nil {
		return nil
	}
	var err = fmt.Errorf("compose err ")

	for _, e := range errs {
		err = fmt.Errorf("%s,%s", err, e)
	}

	return err
}

func SaveToDisk(i media.Writer, track *webrtc.TrackRemote) error {
	defer func() {
		if err := i.Close(); err != nil {
			fmt.Println("close saver err:", err)
		}
	}()

	for {

		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			return err
		}
		if err := i.WriteRTP(rtpPacket); err != nil {
			//fmt.Println(err, "\n", rtpPacket.String())
			return err
		}
	}
}
