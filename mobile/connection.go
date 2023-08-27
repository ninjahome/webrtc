package webrtcLib

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sync/atomic"
)

const (
	IceUdpMtu      = 1 << 10
	FrameStackSize = 4
)

var (
	NCInvalidVideoFrame = fmt.Errorf("invalid ninja video frame info")
	NCOneBadFrameData   = fmt.Errorf("bad frame data")
)

type NinjaConn interface {
	IsConnected() bool
	Close()
	SetRemoteDesc(string) error
}
type H264Conn struct {
	connReader   io.Reader
	connWriter   io.Writer
	frameCounter atomic.Uint32
	Frames       [FrameStackSize]*ReceiveFrame
}

func NewH264Conn(reader io.Reader, writer io.Writer) *H264Conn {
	return &H264Conn{
		connReader: reader,
		connWriter: writer,
	}
}

type VideoFrame struct {
	FrameID    uint16
	SliceCount uint16
	CurSliceID uint16
	CurLen     uint16
}

func (f *VideoFrame) Data() []byte {
	var frameBuf = make([]byte, 8)
	binary.BigEndian.PutUint16(frameBuf[:2], f.FrameID)
	binary.BigEndian.PutUint16(frameBuf[2:4], f.SliceCount)
	binary.BigEndian.PutUint16(frameBuf[4:6], f.CurSliceID)
	binary.BigEndian.PutUint16(frameBuf[6:8], f.CurLen)
	return frameBuf
}
func (f *VideoFrame) SizeInBytes() int {
	return 8
}

func (f *VideoFrame) String() any {
	var s = fmt.Sprintf("{frame id:%d\t", f.FrameID)
	s = s + fmt.Sprintf("slice count:%d\t", f.SliceCount)
	s = s + fmt.Sprintf("current slice:%d\t", f.CurSliceID)
	s = s + fmt.Sprintf("payload:%d}", f.CurLen)
	return s
}

type Slice struct {
	Header  *VideoFrame
	Payload []byte
}
type ReceiveFrame struct {
	SliceHasGot uint16
	HasFinished bool
	Cache       []*Slice
}

func (rf *ReceiveFrame) Flush() []byte {
	var buf []byte
	for _, slice := range rf.Cache {
		buf = append(buf, slice.Payload...)
	}
	return buf
}

func (rf *ReceiveFrame) String() any {
	var s = fmt.Sprintf("{slice got:%d\t", rf.SliceHasGot)
	s = s + fmt.Sprintf("has finished:%t\t", rf.HasFinished)
	s = s + fmt.Sprintf("cache slice:%d}", len(rf.Cache))
	return s
}

func ParseFrame(frame *VideoFrame, data []byte) error {
	frame.FrameID = binary.BigEndian.Uint16(data[:2])
	frame.SliceCount = binary.BigEndian.Uint16(data[2:4])
	frame.CurSliceID = binary.BigEndian.Uint16(data[4:6])
	frame.CurLen = binary.BigEndian.Uint16(data[6:8])
	if frame.CurLen > IceUdpMtu ||
		frame.SliceCount == 0 ||
		frame.CurSliceID > frame.SliceCount {
		fmt.Println("======>>>", frame.String(), hex.EncodeToString(data))
		return NCInvalidVideoFrame
	}
	return nil
}

func (tc *H264Conn) readFrame() (*Slice, error) {

	var frame = &VideoFrame{}
	var frameSizeInBytes = frame.SizeInBytes()
	var buf = make([]byte, IceUdpMtu+frameSizeInBytes)
	var n, err = tc.connReader.Read(buf)
	if err != nil || n < frameSizeInBytes {
		return nil, fmt.Errorf("slice header err: %v-%d", err, n)
	}
	fmt.Println("******>>> tlv got:", hex.EncodeToString(buf[:n]))

	err = ParseFrame(frame, buf[:frameSizeInBytes])
	if err != nil {
		return nil, err
	}
	fmt.Println("******>>> tlv frame:", frame.String())

	buf = buf[frameSizeInBytes:n]
	var sliceLen = len(buf)
	if sliceLen != int(frame.CurLen) {
		return nil, NCOneBadFrameData
	}
	return &Slice{
		frame,
		buf,
	}, nil
}

func (tc *H264Conn) LoopRead(buffer chan []byte) error {

	for {
		var slice, err = tc.readFrame()
		if err != nil {
			return err
		}

		var curIdx = slice.Header.FrameID % FrameStackSize
		var preIdx = (curIdx + FrameStackSize - 1) % FrameStackSize
		var preFrame = tc.Frames[preIdx]
		if preFrame != nil && preFrame.HasFinished {
			buffer <- preFrame.Flush()
			tc.Frames[preIdx] = nil
			fmt.Println("******>>>push previous frame:", preFrame.String())
		}

		var curFrame = tc.Frames[curIdx]
		if curFrame == nil {
			fmt.Println("******>>> create new receive frame for slice:",
				slice.Header.String())
			curFrame = &ReceiveFrame{
				Cache: make([]*Slice, slice.Header.SliceCount),
			}
			tc.Frames[curIdx] = curFrame
		}
		if len(curFrame.Cache) < int(slice.Header.CurSliceID) {
			fmt.Println("*******>>>bad frame slice data:\t", curFrame.String(), slice.Header.String())
			return NCInvalidVideoFrame
		}

		curFrame.Cache[slice.Header.CurSliceID] = slice
		curFrame.SliceHasGot = curFrame.SliceHasGot + 1

		if curFrame.SliceHasGot < slice.Header.SliceCount {
			fmt.Println("******>>> read more slice:",
				curFrame.String(), slice.Header.String())
			continue
		}

		curFrame.HasFinished = true
		fmt.Println("******>>>one frame finished:", curIdx, curFrame.String())
		buffer <- curFrame.Flush()
		tc.Frames[curIdx] = nil
	}
}

func (tc *H264Conn) WriteVideoFrame(buf []byte) (n int, err error) {
	tc.frameCounter.Add(1)
	var dataLen = len(buf)
	fmt.Println("======>>> tlv need to write ", dataLen, hex.EncodeToString(buf))

	var sliceSize = dataLen / IceUdpMtu
	if dataLen%IceUdpMtu > 0 {
		sliceSize = sliceSize + 1
	}
	var frame = &VideoFrame{
		FrameID:    uint16(tc.frameCounter.Load()),
		SliceCount: uint16(sliceSize),
	}

	var sequence uint16 = 0
	for startIdx := 0; startIdx < dataLen; startIdx = startIdx + IceUdpMtu {
		var endIdx = startIdx + IceUdpMtu
		var sliceLen = IceUdpMtu
		if endIdx > dataLen {
			endIdx = dataLen
			sliceLen = dataLen - startIdx
		}
		frame.CurSliceID = sequence
		frame.CurLen = uint16(sliceLen)

		var dataToWrite = frame.Data()
		dataToWrite = append(dataToWrite, buf[startIdx:endIdx]...)

		n, err = tc.connWriter.Write(dataToWrite)
		if err != nil || n == 0 {
			return
		}
		fmt.Println("======>>> tlv write ", n, frame.String(), hex.EncodeToString(dataToWrite))
		sequence = sequence + 1
	}

	return dataLen, nil
}

func FrameWrite(source func() ([]byte, error), conn io.Writer) error {
	var writer = &H264Conn{connWriter: conn}
	for {
		var data, err = source()
		if err != nil {
			return err
		}
		fmt.Println("======>>>camera frame:", hex.EncodeToString(data))
		var _, errW = writer.WriteVideoFrame(data)
		if errW != nil {
			return errW
		}
	}
}
