GO ?= go
BIN := git-k
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: run build test vet fmt tidy icon clean

# 开发运行：始终关闭 cgo，mygo 的 native UI 不需要它。
run:
	CGO_ENABLED=0 $(GO) run . -d test-repo

# 静态二进制就是发布物；生产构建去掉 inspector。
build:
	CGO_ENABLED=0 $(GO) build -trimpath -tags mygo_noinspector \
		-ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) .

test:
	CGO_ENABLED=0 $(GO) test ./...

vet:
	CGO_ENABLED=0 $(GO) vet ./...

fmt:
	$(GO) fmt ./...

tidy:
	CGO_ENABLED=0 $(GO) mod tidy

# 由 assets/icon.svg 的同一几何生成 assets/icon-{256,512}.png。
icon:
	CGO_ENABLED=0 $(GO) run ./tools/genicon

clean:
	rm -f $(BIN)
