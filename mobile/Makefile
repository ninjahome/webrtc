BINDIR=bin

.PHONY:  android ios test

all: android ios test

COMMIT := $(shell git rev-parse HEAD)
VERSION ?= $(shell git describe --tags ${COMMIT} 2> /dev/null || echo "$(COMMIT)")
BUILD_TIME := $(shell LANG=en_US date +"%F_%T_%z")
ROOT := github.com/ninjahome/webrtc/utils
LD_FLAGS := -X $(ROOT).Version=$(VERSION) -X $(ROOT).Commit=$(COMMIT) -X $(ROOT).BuildTime=$(BUILD_TIME)


tp:=./bin

test:
	go build  -ldflags '-w -s' -o $(BINDIR)/ctest mac/*.go

android:
	gomobile bind -v -o $(BINDIR)/webrtcLib.aar -target=android -ldflags="$(LD_FLAGS)" github.com/ninjahome/webrtc/mobile

ios:pbs
	gomobile bind -x -v -trimpath -work -o $(BINDIR)/webrtcLib.xcframework -target=ios  -ldflags="$(LD_FLAGS)"  github.com/ninjahome/webrtc/mobile

sim:pbs
	gomobile bind -v -o $(BINDIR)/webrtcLib.xcframework -ldflags="$(LD_FLAGS)" -target=iossimulator github.com/ninjahome/webrtc/mobile

dev:pbs
	gomobile bind -v -o $(BINDIR)/webrtcLib.xcframework -target=ios/arm64  -ldflags="$(LD_FLAGS)" github.com/ninjahome/webrtc/mobile

pbs:
	echo "--go_out=paths=source_relative:. -I. ./mobile/prortoMsg.proto"

clean:
	gomobile clean
	rm $(BINDIR)/*
