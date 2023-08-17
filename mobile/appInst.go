package webrtcLib

import (
	"sync"
)

type AppInst struct {
	appLocker sync.Locker
}

var _inst = &AppInst{}
