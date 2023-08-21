package webrtcLib

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/nareix/joy4/codec/h264parser"
	"github.com/ninjahome/webrtc/utils"
	"github.com/pion/webrtc/v3"
	"testing"
)

func TestParser(t *testing.T) {
	var typ int
	var nalus [][]byte

	annexbFrame, _ := hex.DecodeString("000000012742001fab405a050c80")
	fmt.Println(annexbFrame)
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
	var offer = `eyJ0eXBlIjoib2ZmZXIiLCJzZHAiOiJ2PTBcclxubz0tIDI0Mzk0MTU0NDEwNDU2MzMwMCAxNjkyNDMxODc1IElOIElQNCAwLjAuMC4wXHJcbnM9LVxyXG50PTAgMFxyXG5hPWZpbmdlcnByaW50OnNoYS0yNTYgQTE6MDU6RTk6MUI6NDE6RTI6N0I6NkU6MkY6NTM6NjU6QjI6NDU6ODU6MTQ6NEY6QUY6MDg6OTc6RDM6Qzc6N0I6M0M6RTQ6MUQ6Njk6NDM6MEI6QjE6NEU6NDM6MTdcclxuYT1leHRtYXAtYWxsb3ctbWl4ZWRcclxuYT1ncm91cDpCVU5ETEUgMCAxIDIgM1xyXG5tPXZpZGVvIDkgVURQL1RMUy9SVFAvU0FWUEYgMTI1XHJcbmM9SU4gSVA0IDAuMC4wLjBcclxuYT1zZXR1cDphY3RwYXNzXHJcbmE9bWlkOjBcclxuYT1pY2UtdWZyYWc6cmpTVmxWeVhid21yUFNlV1xyXG5hPWljZS1wd2Q6SEtDT3FHb0FIV3NLd3B4RFFPYmJUTXRiUFBrYmFzTnZcclxuYT1ydGNwLW11eFxyXG5hPXJ0Y3AtcnNpemVcclxuYT1ydHBtYXA6MTI1IEgyNjQvOTAwMDBcclxuYT1mbXRwOjEyNSBsZXZlbC1hc3ltbWV0cnktYWxsb3dlZD0xO3BhY2tldGl6YXRpb24tbW9kZT0xO3Byb2ZpbGUtbGV2ZWwtaWQ9NDJlMDFmXHJcbmE9c3NyYzo2NzA5OTIzOTEgY25hbWU6aGxZcnBoaFdmdm5ZeEx0TlxyXG5hPXNzcmM6NjcwOTkyMzkxIG1zaWQ6aGxZcnBoaFdmdm5ZeEx0TiBXS3VVenduZ2d6cUJJSkhQXHJcbmE9c3NyYzo2NzA5OTIzOTEgbXNsYWJlbDpobFlycGhoV2Z2bll4THROXHJcbmE9c3NyYzo2NzA5OTIzOTEgbGFiZWw6V0t1VXp3bmdnenFCSUpIUFxyXG5hPW1zaWQ6aGxZcnBoaFdmdm5ZeEx0TiBXS3VVenduZ2d6cUJJSkhQXHJcbmE9c2VuZHJlY3ZcclxuYT1jYW5kaWRhdGU6MzAwNzIyMzMzMyAxIHVkcCAyMTMwNzA2NDMxIDE5Mi4xNjguMS4xMDYgNTcyMDIgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MzAwNzIyMzMzMyAyIHVkcCAyMTMwNzA2NDMxIDE5Mi4xNjguMS4xMDYgNTcyMDIgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjAzMjQxNTkzMyAxIHVkcCAyMTMwNzA2NDMxIDE2OS4yNTQuMTgxLjIwOSA1Njk5NSB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToyMDMyNDE1OTMzIDIgdWRwIDIxMzA3MDY0MzEgMTY5LjI1NC4xODEuMjA5IDU2OTk1IHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjIyODU3MDU4MjkgMSB1ZHAgMTY5NDQ5ODgxNSAyMTguMjQxLjIyMC41OCA2MTI3MyB0eXAgc3JmbHggcmFkZHIgMC4wLjAuMCBycG9ydCA2MTI3M1xyXG5hPWNhbmRpZGF0ZToyMjg1NzA1ODI5IDIgdWRwIDE2OTQ0OTg4MTUgMjE4LjI0MS4yMjAuNTggNjEyNzMgdHlwIHNyZmx4IHJhZGRyIDAuMC4wLjAgcnBvcnQgNjEyNzNcclxuYT1lbmQtb2YtY2FuZGlkYXRlc1xyXG5tPWF1ZGlvIDkgVURQL1RMUy9SVFAvU0FWUEYgMTExXHJcbmM9SU4gSVA0IDAuMC4wLjBcclxuYT1zZXR1cDphY3RwYXNzXHJcbmE9bWlkOjFcclxuYT1pY2UtdWZyYWc6cmpTVmxWeVhid21yUFNlV1xyXG5hPWljZS1wd2Q6SEtDT3FHb0FIV3NLd3B4RFFPYmJUTXRiUFBrYmFzTnZcclxuYT1ydGNwLW11eFxyXG5hPXJ0Y3AtcnNpemVcclxuYT1ydHBtYXA6MTExIG9wdXMvNDgwMDAvMlxyXG5hPWZtdHA6MTExIG1pbnB0aW1lPTEwO3VzZWluYmFuZGZlYz0xXHJcbmE9c3NyYzozMzI5ODUxMjc4IGNuYW1lOkVCZUlYUXZyS2x6SGpJZkJcclxuYT1zc3JjOjMzMjk4NTEyNzggbXNpZDpFQmVJWFF2cktsekhqSWZCIHFIU3hqU1BxakxTUGVGZU1cclxuYT1zc3JjOjMzMjk4NTEyNzggbXNsYWJlbDpFQmVJWFF2cktsekhqSWZCXHJcbmE9c3NyYzozMzI5ODUxMjc4IGxhYmVsOnFIU3hqU1BxakxTUGVGZU1cclxuYT1tc2lkOkVCZUlYUXZyS2x6SGpJZkIgcUhTeGpTUHFqTFNQZUZlTVxyXG5hPXNlbmRyZWN2XHJcbm09dmlkZW8gOSBVRFAvVExTL1JUUC9TQVZQRiAxMjVcclxuYz1JTiBJUDQgMC4wLjAuMFxyXG5hPXNldHVwOmFjdHBhc3NcclxuYT1taWQ6MlxyXG5hPWljZS11ZnJhZzpyalNWbFZ5WGJ3bXJQU2VXXHJcbmE9aWNlLXB3ZDpIS0NPcUdvQUhXc0t3cHhEUU9iYlRNdGJQUGtiYXNOdlxyXG5hPXJ0Y3AtbXV4XHJcbmE9cnRjcC1yc2l6ZVxyXG5hPXJ0cG1hcDoxMjUgSDI2NC85MDAwMFxyXG5hPWZtdHA6MTI1IGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTE7cHJvZmlsZS1sZXZlbC1pZD00MmUwMWZcclxuYT1zc3JjOjM5ODUwMTU4NDcgY25hbWU6NzMzM2Y0M2QtZjFiOC00MjU5LWE5MDAtNWNlYTM1Yzc0MTBiXHJcbmE9c3NyYzozOTg1MDE1ODQ3IG1zaWQ6MDg3MGFiNzEtMTFjZi00YjA0LTk2ZTktMWJmYzA2MjVmNGM4IDM4Zjk5ODRkLWNkN2QtNDdjYy04ODdjLTJlMjViMDM1Y2UwY1xyXG5hPXNzcmM6Mzk4NTAxNTg0NyBtc2xhYmVsOjA4NzBhYjcxLTExY2YtNGIwNC05NmU5LTFiZmMwNjI1ZjRjOFxyXG5hPXNzcmM6Mzk4NTAxNTg0NyBsYWJlbDozOGY5OTg0ZC1jZDdkLTQ3Y2MtODg3Yy0yZTI1YjAzNWNlMGNcclxuYT1tc2lkOjNlY2I3MGRiLTgyMTUtNGRiMS1hNjE3LTFlMGQ4NmMxM2EyNSAzOGY5OTg0ZC1jZDdkLTQ3Y2MtODg3Yy0yZTI1YjAzNWNlMGNcclxuYT1zZW5kcmVjdlxyXG5tPWF1ZGlvIDkgVURQL1RMUy9SVFAvU0FWUEYgMTExXHJcbmM9SU4gSVA0IDAuMC4wLjBcclxuYT1zZXR1cDphY3RwYXNzXHJcbmE9bWlkOjNcclxuYT1pY2UtdWZyYWc6cmpTVmxWeVhid21yUFNlV1xyXG5hPWljZS1wd2Q6SEtDT3FHb0FIV3NLd3B4RFFPYmJUTXRiUFBrYmFzTnZcclxuYT1ydGNwLW11eFxyXG5hPXJ0Y3AtcnNpemVcclxuYT1ydHBtYXA6MTExIG9wdXMvNDgwMDAvMlxyXG5hPWZtdHA6MTExIG1pbnB0aW1lPTEwO3VzZWluYmFuZGZlYz0xXHJcbmE9c3NyYzozNTAzMDYxMzk2IGNuYW1lOjI2MmFlNzEwLWI0OTktNDAzZC04MTdlLTE3ZWM0NDYyYzIzZlxyXG5hPXNzcmM6MzUwMzA2MTM5NiBtc2lkOjVkYWM0ODBmLTBlMjItNDhjOC1hNzhiLTlmMzhiNTEwOWVmNiA0ZWFlZjQwMC0wMzQ5LTQ0OTAtOTI2Zi03NjI3MjU5NzI3ZGFcclxuYT1zc3JjOjM1MDMwNjEzOTYgbXNsYWJlbDo1ZGFjNDgwZi0wZTIyLTQ4YzgtYTc4Yi05ZjM4YjUxMDllZjZcclxuYT1zc3JjOjM1MDMwNjEzOTYgbGFiZWw6NGVhZWY0MDAtMDM0OS00NDkwLTkyNmYtNzYyNzI1OTcyN2RhXHJcbmE9bXNpZDozNGFjOGEyZC0wNTg4LTRhNmYtOWI2NC1lODQ5NDZjYmYxNTQgNGVhZWY0MDAtMDM0OS00NDkwLTkyNmYtNzYyNzI1OTcyN2RhXHJcbmE9c2VuZHJlY3ZcclxuIn0=`
	var offer_sdp = &webrtc.SessionDescription{}
	utils.Decode(offer, offer_sdp)
	fmt.Println(offer_sdp)

	var anwser = `eyJ0eXBlIjoiYW5zd2VyIiwic2RwIjoidj0wXHJcbm89LSA4MzgwNzU1OTk4OTMwOTI0MDMgMTY5MjQzMjAyNyBJTiBJUDQgMC4wLjAuMFxyXG5zPS1cclxudD0wIDBcclxuYT1maW5nZXJwcmludDpzaGEtMjU2IDRCOjgyOjdBOjZEOkI0OkVCOjVEOjEyOjk4OjE1OkNBOjEzOkQ2Ojg4OkNFOjkwOkE5OjEyOjNCOkIyOkUzOkRGOjk2OjVCOjMyOkQ5OkIyOjNBOjg5OkI2Ojc3OjhBXHJcbmE9ZXh0bWFwLWFsbG93LW1peGVkXHJcbmE9Z3JvdXA6QlVORExFIDAgMlxyXG5tPXZpZGVvIDkgVURQL1RMUy9SVFAvU0FWUEYgMTI1XHJcbmM9SU4gSVA0IDAuMC4wLjBcclxuYT1zZXR1cDphY3RpdmVcclxuYT1taWQ6MFxyXG5hPWljZS11ZnJhZzp0TUxJUWdLWVhicW9jUXNaXHJcbmE9aWNlLXB3ZDpnQ3VJQm5sa25jaGNlblRFcllWSGpSanpkamJOSVBGZVxyXG5hPXJ0Y3AtbXV4XHJcbmE9cnRjcC1yc2l6ZVxyXG5hPXJ0cG1hcDoxMjUgSDI2NC85MDAwMFxyXG5hPWZtdHA6MTI1IGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTE7cHJvZmlsZS1sZXZlbC1pZD00MmUwMWZcclxuYT1zc3JjOjI1NzkyODAxMzQgY25hbWU6bmluamFcclxuYT1zc3JjOjI1NzkyODAxMzQgbXNpZDpuaW5qYSB2aWRlb1xyXG5hPXNzcmM6MjU3OTI4MDEzNCBtc2xhYmVsOm5pbmphXHJcbmE9c3NyYzoyNTc5MjgwMTM0IGxhYmVsOnZpZGVvXHJcbmE9bXNpZDpuaW5qYSB2aWRlb1xyXG5hPXNlbmRyZWN2XHJcbmE9Y2FuZGlkYXRlOjI2Mzc4NDAxMTAgMSB1ZHAgMjEzMDcwNjQzMSAxNjkuMjU0LjIxMy4xMiA2NDUwOCB0eXAgaG9zdFxyXG5hPWNhbmRpZGF0ZToyNjM3ODQwMTEwIDIgdWRwIDIxMzA3MDY0MzEgMTY5LjI1NC4yMTMuMTIgNjQ1MDggdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjA3ODE1MzA0NSAxIHVkcCAyMTMwNzA2NDMxIDE5Mi4xNjguMS4xMDMgNjEwNDQgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjA3ODE1MzA0NSAyIHVkcCAyMTMwNzA2NDMxIDE5Mi4xNjguMS4xMDMgNjEwNDQgdHlwIGhvc3RcclxuYT1jYW5kaWRhdGU6MjI4NTcwNTgyOSAxIHVkcCAxNjk0NDk4ODE1IDIxOC4yNDEuMjIwLjU4IDU4MzcxIHR5cCBzcmZseCByYWRkciAwLjAuMC4wIHJwb3J0IDU4MzcxXHJcbmE9Y2FuZGlkYXRlOjIyODU3MDU4MjkgMiB1ZHAgMTY5NDQ5ODgxNSAyMTguMjQxLjIyMC41OCA1ODM3MSB0eXAgc3JmbHggcmFkZHIgMC4wLjAuMCBycG9ydCA1ODM3MVxyXG5hPWVuZC1vZi1jYW5kaWRhdGVzXHJcbm09YXVkaW8gMCBVRFAvVExTL1JUUC9TQVZQRiAwXHJcbmM9SU4gSVA0IDAuMC4wLjBcclxubT12aWRlbyA5IFVEUC9UTFMvUlRQL1NBVlBGIDEyNVxyXG5jPUlOIElQNCAwLjAuMC4wXHJcbmE9c2V0dXA6YWN0aXZlXHJcbmE9bWlkOjJcclxuYT1pY2UtdWZyYWc6dE1MSVFnS1lYYnFvY1FzWlxyXG5hPWljZS1wd2Q6Z0N1SUJubGtuY2hjZW5URXJZVkhqUmp6ZGpiTklQRmVcclxuYT1ydGNwLW11eFxyXG5hPXJ0Y3AtcnNpemVcclxuYT1ydHBtYXA6MTI1IEgyNjQvOTAwMDBcclxuYT1mbXRwOjEyNSBsZXZlbC1hc3ltbWV0cnktYWxsb3dlZD0xO3BhY2tldGl6YXRpb24tbW9kZT0xO3Byb2ZpbGUtbGV2ZWwtaWQ9NDJlMDFmXHJcbmE9cmVjdm9ubHlcclxubT1hdWRpbyAwIFVEUC9UTFMvUlRQL1NBVlBGIDBcclxuYz1JTiBJUDQgMC4wLjAuMFxyXG4ifQ==`
	var anwser_sdp = &webrtc.SessionDescription{}
	utils.Decode(anwser, anwser_sdp)
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
