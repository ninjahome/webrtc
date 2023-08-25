package webrtcLib

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/nareix/joy4/codec/h264parser"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/webrtc/v3"
	"strings"
	"testing"
)

func TestParser(t *testing.T) {
	var typ int
	var nalus [][]byte

	annexbFrame, _ := hex.DecodeString("000000012742001fab405a050c80")
	fmt.Println(annexbFrame)
	fmt.Println("----->", annexbFrame[4]&0x1F)

	nalus, typ = h264parser.SplitNALUs(annexbFrame)
	fmt.Println(typ, len(nalus))

	avccFrame, _ := hex.DecodeString("0000000128ce3c80")
	fmt.Println(avccFrame)

	nalus, typ = h264parser.SplitNALUs(avccFrame)
	fmt.Println(typ, len(nalus))

	annexbFrame2, _ := hex.DecodeString("")
	fmt.Println(annexbFrame2)
	nalus, typ = h264parser.SplitNALUs(annexbFrame2)
	fmt.Println(typ, len(nalus))

	fmt.Println(annexbFrame2[0] & 0x1F)
	fmt.Println(annexbFrame2[1] & 0x40)

}
func TestCodec(t *testing.T) {
	var offer = `eyJjYW5kaWRhdGVzIjpudWxsLCJmcmFnIjoiV3lqVGJWR2pGYmhBYlRISCIsInB3ZCI6ImNSVUhPU3Fra2ZlWWdYdW5KYUVOb0trRmlYd1FVd0NsIn0=`
	var param = &IceConnParam{}
	var err = utils.Decode(offer, param)
	if err != nil {
		t.Fatal(err)
	}

	var offer_sdp = &webrtc.SessionDescription{}
	err = utils.Decode(offer, offer_sdp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(offer_sdp)

	var anwser = `eyJ0eXBlIjoiYW5zd2VyIiwic2RwIjoidj0wXHJcbm89LSAyMjk2NjQ3ODA2MDM0OTkyNDM1IDE2OTI5MzE2ODMgSU4gSVA0IDAuMC4wLjBcclxucz0tXHJcbnQ9MCAwXHJcbmE9ZmluZ2VycHJpbnQ6c2hhLTI1NiA5NDo0QjpCMzpBOTo1Qjo0OTo2MTo0Qzo0MTpERjo2Qzo3NDo1ODozRTpFNTo3Rjo5NjpENzo0RTo3Mjo3RTo4ODpGNToxRDoxNjo2QzpBRTo0RDpCNzo3Qzo0RjpCMFxyXG5hPWV4dG1hcC1hbGxvdy1taXhlZFxyXG5hPWdyb3VwOkJVTkRMRSAwXHJcbm09dmlkZW8gOSBVRFAvVExTL1JUUC9TQVZQRiAxMjVcclxuYz1JTiBJUDQgMC4wLjAuMFxyXG5hPXNldHVwOmFjdGl2ZVxyXG5hPW1pZDowXHJcbmE9aWNlLXVmcmFnOnFnYkVhbktKTXJLYnVudUxcclxuYT1pY2UtcHdkOkprZ213YnVxTHB6Sm1Nc3N1Y2p2VkluUW9ra2VnSGVtXHJcbmE9cnRjcC1tdXhcclxuYT1ydGNwLXJzaXplXHJcbmE9cnRwbWFwOjEyNSBIMjY0LzkwMDAwXHJcbmE9Zm10cDoxMjUgbGV2ZWwtYXN5bW1ldHJ5LWFsbG93ZWQ9MTtwYWNrZXRpemF0aW9uLW1vZGU9MTtwcm9maWxlLWxldmVsLWlkPTQyZTAxZlxyXG5hPXNzcmM6MTQ5MTQ4MDcgY25hbWU6bmluamEtdmlkZW9cclxuYT1zc3JjOjE0OTE0ODA3IG1zaWQ6bmluamEtdmlkZW8gdmlkZW9cclxuYT1zc3JjOjE0OTE0ODA3IG1zbGFiZWw6bmluamEtdmlkZW9cclxuYT1zc3JjOjE0OTE0ODA3IGxhYmVsOnZpZGVvXHJcbmE9bXNpZDpuaW5qYS12aWRlbyB2aWRlb1xyXG5hPXNlbmRyZWN2XHJcbmE9Y2FuZGlkYXRlOjEzNjU1NjY5OTUgMSB1ZHAgMjEzMDcwNjQzMSAxMC44OC4yMDEuOTQgNTE1MDkgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MTM2NTU2Njk5NSAyIHVkcCAyMTMwNzA2NDMxIDEwLjg4LjIwMS45NCA1MTUwOSB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZTo2MjgwNjU1MDkgMSB1ZHAgMjEzMDcwNjQzMSAxNjkuMjU0LjI0Ni4yMzggNTUyNjUgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6NjI4MDY1NTA5IDIgdWRwIDIxMzA3MDY0MzEgMTY5LjI1NC4yNDYuMjM4IDU1MjY1IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjE4NzYyMjkzIDEgdWRwIDIxMzA3MDY0MzEgMTkyLjE2OC4xLjEwMSA0OTU0OCB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToxODc2MjI5MyAyIHVkcCAyMTMwNzA2NDMxIDE5Mi4xNjguMS4xMDEgNDk1NDggdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MTkwNDQzNTU5OSAxIHVkcCAyMTMwNzA2NDMxIDEwLjAuMC44IDU4NTc5IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjE5MDQ0MzU1OTkgMiB1ZHAgMjEzMDcwNjQzMSAxMC4wLjAuOCA1ODU3OSB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToyMjg1NzA1ODI5IDEgdWRwIDE2OTQ0OTg4MTUgMjE4LjI0MS4yMjAuNTggNTYzMzcgdHlwIHNyZmx4IHJhZGRyIDAuMC4wLjAgcnBvcnQgNjQ0MzRcclxuYT1jYW5kaWRhdGU6MjI4NTcwNTgyOSAyIHVkcCAxNjk0NDk4ODE1IDIxOC4yNDEuMjIwLjU4IDU2MzM3IHR5cCBzcmZseCByYWRkciAwLjAuMC4wIHJwb3J0IDY0NDM0XHJcbmE9Y2FuZGlkYXRlOjE3MjQxOTAzODcgMSB1ZHAgMjEzMDcwNjQzMSAyNDA4Ojg1MDc6Nzk2MDo2NWU5OjQyMTo3M2M4OjEwMDI6OWY3OCA2NDg5NiB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToxNzI0MTkwMzg3IDIgdWRwIDIxMzA3MDY0MzEgMjQwODo4NTA3Ojc5NjA6NjVlOTo0MjE6NzNjODoxMDAyOjlmNzggNjQ4OTYgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjcyOTU4NzkgMSB1ZHAgMjEzMDcwNjQzMSAyNDA4Ojg1MDc6Nzk2MDo2NWU5OjU4ZmQ6MWZiMjplYTRiOmI5ZmQgNTM4NDAgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjcyOTU4NzkgMiB1ZHAgMjEzMDcwNjQzMSAyNDA4Ojg1MDc6Nzk2MDo2NWU5OjU4ZmQ6MWZiMjplYTRiOmI5ZmQgNTM4NDAgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MTEyMTE2ODk0MCAxIHVkcCAyMTMwNzA2NDMxIDI0MDk6ODkwMDoxZjk2OjQwNDpjZWU6MzBjNDplNGE2OmJmNDQgNTY1NDcgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MTEyMTE2ODk0MCAyIHVkcCAyMTMwNzA2NDMxIDI0MDk6ODkwMDoxZjk2OjQwNDpjZWU6MzBjNDplNGE2OmJmNDQgNTY1NDcgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MTY3Nzg2NzkyNSAxIHVkcCAyMTMwNzA2NDMxIDI0MDk6ODkwMDoxZjk2OjQwNDozOWY0OmNlYzE6OWY2ZDpiNzJkIDU1Njc2IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjE2Nzc4Njc5MjUgMiB1ZHAgMjEzMDcwNjQzMSAyNDA5Ojg5MDA6MWY5Njo0MDQ6MzlmNDpjZWMxOjlmNmQ6YjcyZCA1NTY3NiB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToyNzE3MzMwNzg1IDEgdWRwIDIxMzA3MDY0MzEgMjQwOTo4MTAwOjdmOTY6NDlkNToxODMwOmI0OWM6MmMzZDplNjYyIDQ5Njc0IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjI3MTczMzA3ODUgMiB1ZHAgMjEzMDcwNjQzMSAyNDA5OjgxMDA6N2Y5Njo0OWQ1OjE4MzA6YjQ5YzoyYzNkOmU2NjIgNDk2NzQgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6ODMzNzgwMTcyIDEgdWRwIDIxMzA3MDY0MzEgMjQwOTo4MTAwOjdmOTY6NDlkNTpmZGE0OjIzNmY6OGJhNjo1N2UzIDYxODQ0IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjgzMzc4MDE3MiAyIHVkcCAyMTMwNzA2NDMxIDI0MDk6ODEwMDo3Zjk2OjQ5ZDU6ZmRhNDoyMzZmOjhiYTY6NTdlMyA2MTg0NCB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToyNzE3MzMwNzg1IDEgdWRwIDIxMzA3MDY0MzEgMjQwOTo4MTAwOjdmOTY6NDlkNToxODMwOmI0OWM6MmMzZDplNjYyIDU0NzY0IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjI3MTczMzA3ODUgMiB1ZHAgMjEzMDcwNjQzMSAyNDA5OjgxMDA6N2Y5Njo0OWQ1OjE4MzA6YjQ5YzoyYzNkOmU2NjIgNTQ3NjQgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjcxNzMzMDc4NSAxIHVkcCAyMTMwNzA2NDMxIDI0MDk6ODEwMDo3Zjk2OjQ5ZDU6MTgzMDpiNDljOjJjM2Q6ZTY2MiA0OTQyNyB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToyNzE3MzMwNzg1IDIgdWRwIDIxMzA3MDY0MzEgMjQwOTo4MTAwOjdmOTY6NDlkNToxODMwOmI0OWM6MmMzZDplNjYyIDQ5NDI3IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjE3MjQxOTAzODcgMSB1ZHAgMjEzMDcwNjQzMSAyNDA4Ojg1MDc6Nzk2MDo2NWU5OjQyMTo3M2M4OjEwMDI6OWY3OCA2MTk5OCB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToxNzI0MTkwMzg3IDIgdWRwIDIxMzA3MDY0MzEgMjQwODo4NTA3Ojc5NjA6NjVlOTo0MjE6NzNjODoxMDAyOjlmNzggNjE5OTggdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MTcyNDE5MDM4NyAxIHVkcCAyMTMwNzA2NDMxIDI0MDg6ODUwNzo3OTYwOjY1ZTk6NDIxOjczYzg6MTAwMjo5Zjc4IDYzMjA4IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjE3MjQxOTAzODcgMiB1ZHAgMjEzMDcwNjQzMSAyNDA4Ojg1MDc6Nzk2MDo2NWU5OjQyMTo3M2M4OjEwMDI6OWY3OCA2MzIwOCB0eXAgaG9zdFxyXG5hPWVuZC1vZi1jYW5kaWRhdGVzXHJcbiJ9`
	var anwser_sdp = &webrtc.SessionDescription{}
	err = utils.Decode(anwser, anwser_sdp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(anwser_sdp)
}

func TestBigEndian(t *testing.T) {
	bts, err := hex.DecodeString("5c819b20")
	if err != nil {
		t.Fail()
	}
	fmt.Println(bts)
	var buf = make([]byte, 4)
	binary.BigEndian.PutUint32(buf, 1188)
	fmt.Println(binary.BigEndian.Uint32(buf))
	fmt.Println(binary.LittleEndian.Uint32(buf))
	fmt.Println(hex.EncodeToString(buf))
	fmt.Println((buf))

	binary.LittleEndian.PutUint32(buf, 1188)
	fmt.Println(binary.BigEndian.Uint32(buf))
	fmt.Println(binary.LittleEndian.Uint32(buf))
	fmt.Println(hex.EncodeToString(buf))
	fmt.Println((buf))
}
func TestSpsTest(t *testing.T) {
	fmt.Println(0x41 & 0x1F)
}

func TestReader(t *testing.T) {
	var str = `000000016742c028da0280f684000003000400000300ca3c60ca800000000168ce3c80`
	var bts, _ = hex.DecodeString(str)

	_, _ = h254Write(bts, func(typ int, h264data []byte) {
		fmt.Println(typ, hex.EncodeToString(h264data))
	})

	str = `00000001658882017a0c6002ae1600b599200d00014416c02a627d57b4d7e4d05c78a51880b1184022d3808f074310f3be83e2e366a4183ed8f92a12174c205e1c600021850e10000850c6794f2de5bc0d904025d929183861403700580595500b0121cc`
	bts, _ = hex.DecodeString(str)

	_, _ = h254Write(bts, func(typ int, h264data []byte) {
		fmt.Println(typ, hex.EncodeToString(h264data))
	})

	str = `00000001410192688042bc562c74324b491f84a77e77b76ad784b37b74db6fde5afc277d37bfc94865e88a17822b2347237cb9de89eef08595a3499e9236d0efe4d8e395f8ba2619a5cef3ba8fc75898988eb9d85b73a506f60ec319f086a6b67aa57f84`
	bts, _ = hex.DecodeString(str)

	_, _ = h254Write(bts, func(typ int, h264data []byte) {
		fmt.Println(typ, hex.EncodeToString(h264data))
	})

}
func TestIndex(t *testing.T) {
	var str = `000000012742001fab405a050c80`
	var bts, _ = hex.DecodeString(str)
	fmt.Println(bts)
	var idx = bytes.Index(bts, startCode)
	fmt.Println(bts[idx+sCodeLen] & H264TypMask)
	for {
		var idx = bytes.Index(bts, startCode)
		if idx != -1 {
			bts = bts[idx+sCodeLen:]
			fmt.Println(bts)
		} else {
			fmt.Println(bts)
			break
		}
	}
}

func TestLittleEndian(t *testing.T) {
	var str = `000000016742c028da0280f684000003000400000300ca3c60ca800000000168ce3c80`
	var bts, _ = hex.DecodeString(str)
	fmt.Println(binary.BigEndian.Uint32(bts[:4]))
	fmt.Println(binary.LittleEndian.Uint32(bts[:4]))
	var spsorpps = strings.Split(str, "00000001")
	fmt.Println(spsorpps)
	var cache bytes.Buffer
	cache.Write(bts)
	for {
		var data, er = cache.ReadBytes(1)
		fmt.Println(data, cache.Bytes())
		if er != nil {
			break
		}
	}
}
