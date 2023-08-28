package webrtcLib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

const (
	QCHeaderSize  = 4
	QCNodePool    = 1 << 6
	QCNullPointer = -1
)

var (
	QCErrDataLost   = fmt.Errorf("data lost")
	QCErrHeaderLost = fmt.Errorf("header lost")
)

type QueueConn struct {
	reader io.Reader
	writer io.Writer
	seq    uint32

	nodePool *sortedQueue
	sig      chan struct{}
}

type dataNode struct {
	seq   uint32
	buf   []byte
	isKey bool
}

func (dn *dataNode) String() string {
	var s = fmt.Sprintf("{seq:%d\t", dn.seq)
	s += fmt.Sprintf("isKey:%t\t", dn.isKey)
	s += fmt.Sprintf("buf:%d}", len(dn.buf))
	return s
}

func (dn *dataNode) isEmpty() bool {
	return dn.seq == 0 || len(dn.buf) == 0
}

func NewQueueConn(reader io.Reader, writer io.Writer) *QueueConn {
	return &QueueConn{
		reader: reader,
		writer: writer,
		nodePool: &sortedQueue{
			pointer: QCNullPointer,
			pool:    make([]*dataNode, QCNodePool),
		},
		sig: make(chan struct{}, QCNodePool),
	}
}

func (qc *QueueConn) writeFrameData(buf []byte) (n int, err error) {

	var dataLen = len(buf)

	for startIdx := 0; startIdx < dataLen; startIdx = startIdx + IceUdpMtu {
		var endIdx = startIdx + IceUdpMtu
		var sliceLen = IceUdpMtu
		if endIdx > dataLen {
			endIdx = dataLen
			sliceLen = dataLen - startIdx
		}
		var seqBuf = make([]byte, QCHeaderSize)
		qc.seq++
		binary.BigEndian.PutUint32(seqBuf, qc.seq)
		seqBuf = append(seqBuf, buf[startIdx:endIdx]...)
		n, err = qc.writer.Write(seqBuf)
		if err != nil {
			return 0, err
		}
		if n != sliceLen+QCHeaderSize {
			return 0, QCErrDataLost
		}
		fmt.Println("======>>>qc write:", qc.seq, dataLen, sliceLen)
	}

	return dataLen, nil
}

func (qc *QueueConn) readFromNetwork() (*dataNode, error) {

	var buffer = make([]byte, IceUdpMtu+QCHeaderSize)
	var n, err = qc.reader.Read(buffer)
	if err != nil {
		return nil, err
	}
	if n < QCHeaderSize {
		return nil, QCErrHeaderLost
	}

	var seq = binary.BigEndian.Uint32(buffer[:QCHeaderSize])
	var videoStartIdx = bytes.Index(buffer[QCHeaderSize:QCHeaderSize+VideoAvcLen], VideoAvcStart)

	buffer = buffer[QCHeaderSize:n]
	var node = &dataNode{
		seq:   seq,
		isKey: videoStartIdx == 0,
		buf:   buffer,
	}
	fmt.Println("******>>>qc read node:", node.String())
	return node, nil
}

func (qc *QueueConn) reading(sig chan struct{}, eCh chan error) {
	for {
		var node, err = qc.readFromNetwork()
		if err != nil {
			eCh <- err
			return
		}
		qc.nodePool.product(node)
		sig <- struct{}{}
	}
}

func (qc *QueueConn) ReadFrameData(bufCh chan []byte) error {

	var errCh = make(chan error, 1)
	go qc.reading(qc.sig, errCh)

	for {
		select {
		case <-qc.sig:
			var buf = qc.nodePool.consume()
			if buf == nil {
				continue
			}
			bufCh <- buf
		case e := <-errCh:
			return e
		}
	}
}

type sortedQueue struct {
	sync.RWMutex
	pointer int
	pool    []*dataNode
	lost    int
}

func (dp *sortedQueue) consume() []byte {
	dp.RLock()
	if dp.pointer == QCNullPointer {
		fmt.Println("------>>> empty queue:")
		dp.RUnlock()
		return nil
	}
	var cur = dp.pool[dp.pointer]
	if cur == nil {
		fmt.Println("------>>> empty queue:")
		dp.RUnlock()
		return nil
	}

	var nextPos = (dp.pointer + 1) % QCNodePool
	var next = dp.pool[nextPos]
	if next == nil {
		fmt.Println("------>>> no next node:", cur.String())
		dp.RUnlock()
		return nil
	}
	dp.RUnlock()

	dp.Lock()
	defer dp.Unlock()
	if !next.isKey {
		fmt.Println("------>>> merge node:", cur.String(), next.String())
		next.buf = append(cur.buf, next.buf...)
		dp.pool[dp.pointer] = nil
		dp.pointer = nextPos
		return nil
	}
	fmt.Println("------>>> found key node:", cur.String(), next.String())
	dp.pool[dp.pointer] = nil
	dp.pointer = nextPos
	return cur.buf
}

func (dp *sortedQueue) product(node *dataNode) {
	if node == nil {
		return
	}
	dp.Lock()
	defer dp.Unlock()
	var pos = int(node.seq % QCNodePool)
	dp.pool[pos] = node
	if dp.pointer == QCNullPointer {
		dp.pointer = pos
	}
}
