# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: kalgo android ios kalgo-cross evm all test clean
.PHONY: kalgo-linux kalgo-linux-386 kalgo-linux-amd64 kalgo-linux-mips64 kalgo-linux-mips64le
.PHONY: kalgo-linux-arm kalgo-linux-arm-5 kalgo-linux-arm-6 kalgo-linux-arm-7 kalgo-linux-arm64
.PHONY: kalgo-darwin kalgo-darwin-386 kalgo-darwin-amd64
.PHONY: kalgo-windows kalgo-windows-386 kalgo-windows-amd64
.PHONY: deplibs

GOBIN = $(shell pwd)/build/bin
GO ?= latest

kalgo:
	sh build/env.sh go run build/ci.go install ./cmd/kalgo
	@echo "Done building."
	@echo "Run \"$(GOBIN)/kalgo\" to launch kalgo."

all:
	build/env.sh go run build/ci.go install

android:
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/kalgo.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/kalgo.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

lint: ## Run linters.
	build/env.sh go run build/ci.go lint

clean:
	./build/clean_go_build_cache.sh
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/kevinburke/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go get -u github.com/golang/protobuf/protoc-gen-go
	env GOBIN= go install ./cmd/kalabigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

# Cross Compilation Targets (xgo)

kalgo-cross: kalgo-linux kalgo-darwin kalgo-windows kalgo-android kalgo-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-*

kalgo-linux: kalgo-linux-386 kalgo-linux-amd64 kalgo-linux-arm kalgo-linux-mips64 kalgo-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-*

kalgo-linux-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/kalgo
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep 386

kalgo-linux-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/kalgo
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep amd64

kalgo-linux-arm: kalgo-linux-arm-5 kalgo-linux-arm-6 kalgo-linux-arm-7 kalgo-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep arm

kalgo-linux-arm-5:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/kalgo
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep arm-5

kalgo-linux-arm-6:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/kalgo
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep arm-6

kalgo-linux-arm-7:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/kalgo
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep arm-7

kalgo-linux-arm64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/kalgo
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep arm64

kalgo-linux-mips:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/kalgo
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep mips

kalgo-linux-mipsle:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/kalgo
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep mipsle

kalgo-linux-mips64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/kalgo
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep mips64

kalgo-linux-mips64le:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/kalgo
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-linux-* | grep mips64le

kalgo-darwin: kalgo-darwin-386 kalgo-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-darwin-*

kalgo-darwin-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/kalgo
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-darwin-* | grep 386

kalgo-darwin-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/kalgo
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-darwin-* | grep amd64

kalgo-windows: kalgo-windows-386 kalgo-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-windows-*

kalgo-windows-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/kalgo
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-windows-* | grep 386

kalgo-windows-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/kalgo
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/kalgo-windows-* | grep amd64

crypto/ed25519/lib/libsodium.a:
	$(MAKE) -C crypto/ed25519 lib/libsodium.a

deplibs: crypto/ed25519/lib/libsodium.a

clean-deplibs:
	$(MAKE) -C crypto/ed25519 clean

