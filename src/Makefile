# SGIS CLI 멀티플랫폼 빌드
# 산출물: src/bin/ 과 스킬 루트 ../apps/ 두 곳에 동일 배치
# 파일명 컨벤션: sgis-<os>-<arch>[.exe] (release.yml / install.sh 와 일치)
# CGO=0 everywhere — sgis는 sqlite/parquet 미사용이므로 cross-compiler 불필요
#
# 사용법:
#   make build   # 5개 플랫폼 전부 빌드 후 bin/ 과 ../apps/ 에 배치
#   make native  # 현재 플랫폼만 빌드
#   make clean   # 산출물 삭제
#   make test    # CGO_ENABLED=0 go test ./...
#   make vet     # go vet ./...
#   make fmt     # gofmt -l .

BIN     := bin
APPS    := ../apps
VERSION := $(shell cat ../VERSION 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build native clean test vet fmt

build:
	@echo "▶ 멀티플랫폼 빌드 시작 (CGO_ENABLED=0, version=$(VERSION))..."
	@mkdir -p $(BIN) $(APPS)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN)/sgis-darwin-arm64      .
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN)/sgis-darwin-amd64      .
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN)/sgis-linux-amd64       .
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN)/sgis-linux-arm64       .
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN)/sgis-windows-amd64.exe .
	@cp $(BIN)/sgis-darwin-arm64 $(BIN)/sgis-darwin-amd64 $(BIN)/sgis-linux-amd64 $(BIN)/sgis-linux-arm64 $(BIN)/sgis-windows-amd64.exe $(APPS)/
	@echo "✅ 빌드 완료 — src/bin/ 과 ../apps/ 에 배치됨"
	@ls -lh $(BIN)/sgis-*

native:
	@mkdir -p $(BIN) $(APPS)
	$(eval NATIVE_OS := $(shell go env GOOS))
	$(eval NATIVE_ARCH := $(shell go env GOARCH))
	$(eval NATIVE_EXT := $(if $(filter windows,$(NATIVE_OS)),.exe,))
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/sgis-$(NATIVE_OS)-$(NATIVE_ARCH)$(NATIVE_EXT) .
	@cp $(BIN)/sgis-$(NATIVE_OS)-$(NATIVE_ARCH)$(NATIVE_EXT) $(APPS)/
	@echo "✅ 네이티브 빌드 완료 ($(NATIVE_OS)/$(NATIVE_ARCH))"

clean:
	rm -f $(BIN)/sgis-* $(APPS)/sgis-*
	@echo "✅ 산출물 삭제 완료"

test:
	CGO_ENABLED=0 go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .
