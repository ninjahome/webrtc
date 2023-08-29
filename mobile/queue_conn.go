package webrtcLib

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

const (
	QCDataVideo QCDataTye = iota + 1
	QCDataAudio
	QCDataNack
)

const (
	QCIceMtu    = 1 << 13
	QCSliceSize = QCIceMtu - QCHeaderSize

	QCDataTypeLen = 1
	QCSequenceLen = 4
	QCHeaderSize  = QCDataTypeLen + QCSequenceLen

	QCNodePool    = 1 << 10
	QCNullPointer = -1

	QCSliceToWait     = 1 << 3
	QCSliceLostToSkip = 1 << 4
	QCMaxSkipTry      = QCSliceLostToSkip
)

var (
	QCErrDataLost    = fmt.Errorf("data lost")
	QCErrHeaderLost  = fmt.Errorf("header lost")
	QCErrAckLost     = fmt.Errorf("ack lost")
	QCErrDataInvalid = fmt.Errorf("data type unknown")
)

type QCDataTye byte

func (t QCDataTye) String() string {
	switch t {
	case QCDataVideo:
		return "video"
	case QCDataAudio:
		return "audio"
	case QCDataNack:
		return "nack"
	default:
		return "unknown"
	}
}

type QueueConn struct {
	conn net.Conn

	seq uint32

	sendCache [][]byte
	rendBuf   chan []byte

	rcvPool *SortedQueue
	rcvSig  chan struct{}
}

func NewQueueConn(c net.Conn) *QueueConn {
	return &QueueConn{
		conn: c,
		rcvPool: &SortedQueue{
			Pointer: QCNullPointer,
			Pool:    make([]*DataNode, QCNodePool),
		},
		rcvSig:    make(chan struct{}, QCNodePool),
		sendCache: make([][]byte, QCNodePool),
		rendBuf:   make(chan []byte, QCNodePool),
	}
}

func (qc *QueueConn) Close() {
	if qc.conn == nil {
		return
	}
	qc.rcvPool.Reset()
	qc.seq = 0
	_ = qc.conn.Close()
	close(qc.rcvSig)
	close(qc.rendBuf)
	qc.conn = nil
}

func (qc *QueueConn) sendWithSeqAndTyp(typ QCDataTye, buf []byte) error {
	var dataLen = len(buf)
	fmt.Println("======>>>frame data=> type:", typ.String(),
		" length:", dataLen) //, hex.EncodeToString(buf))

	for startIdx := 0; startIdx < dataLen; startIdx = startIdx + QCSliceSize {

		var endIdx = startIdx + QCSliceSize
		var sliceLen = QCSliceSize
		if endIdx > dataLen {
			endIdx = dataLen
			sliceLen = dataLen - startIdx
		}

		atomic.AddUint32(&qc.seq, 1)

		var headBuf = make([]byte, QCHeaderSize)
		binary.BigEndian.PutUint32(headBuf[:QCSequenceLen], qc.seq)
		headBuf[QCSequenceLen] = byte(typ)

		var ackIdx = qc.seq % QCNodePool
		var data = append(headBuf, buf[startIdx:endIdx]...)
		qc.sendCache[ackIdx] = data

		var n, errW = qc.conn.Write(data)
		if errW != nil {
			return errW
		}
		if n != sliceLen+QCHeaderSize {
			fmt.Println("======>>>qc write err:", n, hex.EncodeToString(headBuf), hex.EncodeToString(data), sliceLen, QCHeaderSize)
			return QCErrDataLost
		}
		fmt.Println("======>>>qc write=> seq:", qc.seq,
			" slice size:", sliceLen,
			" header:", hex.EncodeToString(headBuf))
	}

	return nil
}

func (qc *QueueConn) WritingFrame(typ QCDataTye, dataSource func() ([]byte, error)) error {
	var errCh = make(chan error, 2)

	for {
		select {
		case er := <-errCh:
			return er
		case lostData := <-qc.rendBuf:
			var _, errW = qc.conn.Write(lostData)
			if errW != nil {
				return errW
			}
			continue
		default:
		}

		var buf, err = dataSource()
		if err != nil {
			return err
		}
		err = qc.sendWithSeqAndTyp(typ, buf)
		if err != nil {
			return err
		}
	}
}

func (qc *QueueConn) readFromNetwork() (*DataNode, error) {

	var buffer = make([]byte, QCIceMtu)
	var n, err = qc.conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	if n < QCHeaderSize {
		fmt.Println("******>>>read invalid data:", n, hex.EncodeToString(buffer[:n]))
		return nil, QCErrHeaderLost
	}

	var seq = binary.BigEndian.Uint32(buffer[:QCSequenceLen])
	var dataTyp = QCDataTye(buffer[QCSequenceLen:QCHeaderSize][0])
	buffer = buffer[QCHeaderSize:n]

	var node = &DataNode{}

	switch dataTyp {
	case QCDataAudio:
		panic("audio is in progress")
	case QCDataNack:
		node.Typ = QCDataNack
		node.Buf = buffer

	case QCDataVideo:

		node.Typ = QCDataVideo
		node.Buf = buffer
		node.Seq = seq

		var videoStartIdx = bytes.Index(buffer[:VideoAvcLen], VideoAvcStart)
		node.IsKey = videoStartIdx == 0

	default:
		return nil, QCErrDataInvalid
	}

	fmt.Println("******>>>qc read node:", node.String()) //, hex.EncodeToString(node.Buf))
	return node, nil
}

func (qc *QueueConn) resendLostPkt(errCh chan error, node *DataNode) {
	for {
		var dataLen = len(node.Buf)
		if dataLen != QCSequenceLen {
			fmt.Println("===+++>>>reading ack lost:", node.String(),
				hex.EncodeToString(node.Buf))
			errCh <- QCErrAckLost
			return
		}

		var ackIdx = binary.BigEndian.Uint32(node.Buf)
		var idxInCache = ackIdx % QCNodePool

		var buf = qc.sendCache[idxInCache]
		if buf == nil {
			fmt.Println("======>>> lost pkt payload not found:", ackIdx)
			continue
		}

		qc.rendBuf <- buf
	}
}

func (qc *QueueConn) reading(sig chan struct{}, eCh chan error) {
	for {
		var node, err = qc.readFromNetwork()
		if err != nil {
			eCh <- err
			return
		}
		if node.Typ == QCDataVideo {
			_ = qc.rcvPool.Product(node)
			sig <- struct{}{}
			continue
		}
		if node.Typ == QCDataNack {
			qc.resendLostPkt(eCh, node)
			continue
		}
		panic("audio in process")
	}
}

func (qc *QueueConn) ReadFrameData(bufCh chan []byte) error {

	var errCh = make(chan error, 2)
	var lostSeq = make(chan uint32, 32)
	go qc.reading(qc.rcvSig, errCh)

	for {
		select {
		case seq := <-lostSeq:
			fmt.Println("******>>>need to resend seq:", seq)

			var buf = make([]byte, QCSequenceLen)
			binary.BigEndian.PutUint32(buf[:QCSequenceLen], seq)
			var err = qc.sendWithSeqAndTyp(QCDataNack, buf)

			if err != nil {
				return err
			}

			continue
		case <-qc.rcvSig:
			var buf = qc.rcvPool.Consume(lostSeq)
			if buf == nil {
				continue
			}
			fmt.Println("******>>>got remote data:", len(buf))
			bufCh <- buf
		case e := <-errCh:
			return e
		}
	}
}

type DataNode struct {
	Typ   QCDataTye
	Seq   uint32
	Buf   []byte
	IsKey bool
}

func (dn *DataNode) String() string {
	var s = fmt.Sprintf("{Seq:%d\t", dn.Seq)
	s += fmt.Sprintf("Typ:%s\t", dn.Typ.String())
	s += fmt.Sprintf("IsKey:%t\t", dn.IsKey)
	s += fmt.Sprintf("Buf:%d}", len(dn.Buf))
	return s
}

func (dn *DataNode) isEmpty() bool {
	return dn.Seq == 0 || len(dn.Buf) == 0
}

type SortedQueue struct {
	sync.RWMutex
	Pointer int
	Pool    []*DataNode

	lostCounter    int
	timeoutCounter int
}

func (dp *SortedQueue) Reset() {
	dp.Lock()
	dp.Unlock()

	dp.Pool = nil
	dp.Pointer = QCNullPointer
}

func (dp *SortedQueue) skipToNextFrameWithoutLock() {
	dp.lostCounter = 0
	dp.timeoutCounter = 0
	for i := 2; i < QCNodePool; i++ {
		var nextPos = (dp.Pointer + i) % QCNodePool
		var next = dp.Pool[nextPos]
		if next == nil {
			continue
		}
		if !next.IsKey {
			continue
		}
		dp.Pointer = nextPos
		fmt.Println("======>>>skip to next frame:", next.String())
		return
	}
}
func (dp *SortedQueue) Consume(lostSeq chan uint32) []byte {
	dp.RLock()
	if dp.Pointer == QCNullPointer {
		//fmt.Println("------>>> empty queue:")
		dp.RUnlock()
		return nil
	}
	var cur = dp.Pool[dp.Pointer]
	if cur == nil {
		//fmt.Println("------>>> empty queue:")
		dp.RUnlock()
		return nil
	}
	dp.RUnlock()

	dp.Lock()
	defer dp.Unlock()

	var nextPos = (dp.Pointer + 1) % QCNodePool
	var next = dp.Pool[nextPos]
	if next == nil {
		dp.lostCounter++
		dp.timeoutCounter++
		fmt.Println("------>>> no next node :", cur.String(),
			" lost counter:", dp.lostCounter,
			" timeout counter:", dp.timeoutCounter)
		return nil
	}

	if dp.lostCounter > QCSliceToWait {
		if dp.timeoutCounter <= QCSliceLostToSkip {
			fmt.Println("------>>> found lost seq:", cur.Seq+1)
			lostSeq <- cur.Seq + 1
			dp.lostCounter = 0
		} else {
			dp.skipToNextFrameWithoutLock()
		}
	}

	if !next.IsKey {
		fmt.Println("------>>> merge node cur:", cur.String(),
			" next:", next.String())
		next.Buf = append(cur.Buf, next.Buf...)
		next.IsKey = cur.IsKey
		dp.Pool[dp.Pointer] = nil
		dp.Pointer = nextPos
		fmt.Println("------>>> next now:", next.String())
		return nil
	}

	fmt.Println("--------------------------------------->>> key node", cur.String(),
		" next:", next.String())
	if !cur.IsKey {
		dp.skipToNextFrameWithoutLock()
		return nil
	}

	dp.Pool[dp.Pointer] = nil
	dp.Pointer = nextPos
	return cur.Buf
}

func (dp *SortedQueue) Product(node *DataNode) error {
	if node == nil {
		return fmt.Errorf("empty data")
	}
	dp.Lock()
	defer dp.Unlock()
	if dp.Pool == nil {
		return fmt.Errorf("empty pool")
	}

	var pos = int(node.Seq % QCNodePool)
	dp.Pool[pos] = node
	if dp.Pointer == QCNullPointer {
		dp.Pointer = pos
	}
	//fmt.Println("------>>> new node put:", node.String())
	return nil
}
