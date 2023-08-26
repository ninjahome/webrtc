package webrtcLib

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

type NinjaConn interface {
	IsConnected() bool
	Close()
	SetRemoteDesc(string) error
}
type h264Conn struct {
	connReader io.Reader
	connWriter io.Writer
}

func (tc *h264Conn) Read(b *[]byte) (int, error) {
	var buf = make([]byte, IceUdpMtu)

	var hasRead, err = tc.connReader.Read(buf)
	if err != nil || hasRead < LenBufSize {
		return 0, fmt.Errorf("tlv conn read failed:%s-%d", err, hasRead)
	}

	var dataLen = int(binary.BigEndian.Uint32(buf[:4]))
	if dataLen > MaxDataSize {
		return 0, fmt.Errorf("tlv conn too big data:%d\t%s", dataLen, hex.EncodeToString(buf[:hasRead]))
	}

	fmt.Println("======>>>tlv conn need to read:", dataLen, hasRead, hex.EncodeToString(buf[:hasRead]))

	if dataLen+LenBufSize == hasRead {
		*b = buf[LenBufSize:hasRead]
		return hasRead - LenBufSize, nil
	}

	*b = append(*b, buf[LenBufSize:hasRead]...)
	hasRead = hasRead - LenBufSize

	fmt.Println("======>>>tlv conn more time read needed:", hasRead, dataLen) //, hex.EncodeToString(buf))

	for {
		var n, errRead = tc.connReader.Read(buf)
		if errRead != nil || n == 0 {
			fmt.Println("======>>>tlv conn multi read err:", errRead, n)
			return 0, errRead
		}
		fmt.Println("======>>>tlv conn multi reading:", dataLen, hasRead, n)
		hasRead = hasRead + n
		if hasRead > dataLen {
			panic("too big reading buffer")
		}
		*b = append(*b, buf[:n]...)
		if hasRead == dataLen {
			return dataLen, nil
		}
	}
}

func (tc *h264Conn) Write(b []byte) (n int, err error) {
	var lenBuf = make([]byte, LenBufSize)
	var dataLen = len(b)

	binary.BigEndian.PutUint32(lenBuf, uint32(dataLen))
	b = append(lenBuf, b...)
	var needToSend = dataLen + LenBufSize

	for startIdx := 0; startIdx < needToSend; startIdx = startIdx + IceUdpMtu {
		if startIdx+IceUdpMtu > needToSend {
			n, err = tc.connWriter.Write(b[startIdx:needToSend])
		} else {
			n, err = tc.connWriter.Write(b[startIdx : startIdx+IceUdpMtu])
		}
		if err != nil || n == 0 {
			return
		}
	}

	return dataLen, nil
}
