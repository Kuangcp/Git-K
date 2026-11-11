GO ?= go1.27
BIN := git-k
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
RELEASE_VERSION := $(patsubst v%,%,$(VERSION))
DIST := dist

# 发布目标；可覆盖，如 make release RELEASE_PLATFORMS=linux/amd64
RELEASE_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# 发布构建：去掉调试信息与 inspector，mygo 置为 production
RELEASE_FLAGS := -trimpath -buildvcs=false -tags mygo_noinspector
RELEASE_LDFLAGS := -s -w -X db-g/app.buildVersion=$(VERSION) -X db-g/app.releaseBuild=1 -X github.com/egoist/mygo.production=1


# macOS 默认没有 sha256sum。
SHA256 ?= $(shell if command -v sha256sum >/dev/null 2>&1; then echo sha256sum; else echo "shasum -a 256"; fi)

.PHONY: run build release test vet fmt tidy icon clean

# 开发运行：始终关闭 cgo，mygo 的 native UI 不需要它。
run:
	CGO_ENABLED=0 $(GO) run . -d test-repo

# 静态二进制就是发布物；生产构建去掉 inspector。
build:
	CGO_ENABLED=0 $(GO) build -trimpath -tags mygo_noinspector \
		-ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) .

# 交叉编译发布包：每个平台一个归档，外加 SHA256SUMS。
# 例：make release RELEASE_PLATFORMS=linux/amd64,darwin/arm64
release:
	rm -rf $(DIST)
	mkdir -p $(DIST)
	set -eu; \
	for target in $$(printf '%s' '$(RELEASE_PLATFORMS)' | tr ',' ' '); do \
		os=$${target%/*}; arch=$${target#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		name=git-k_$(RELEASE_VERSION)_$${os}_$${arch}; \
		dir=$(DIST)/$$name; \
		mkdir -p "$$dir"; \
		echo "构建 $$name"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -tags mygo_noinspector \
			-ldflags "-s -w -X main.version=$(VERSION)" -o "$$dir/git-k$$ext" .; \
		cp LICENSE Readme.md Readme.en.md "$$dir/"; \
		if [ "$$os" = windows ]; then \
			(cd $(DIST) && zip -qr "$$name.zip" "$$name"); \
		else \
			(cd $(DIST) && tar -czf "$$name.tar.gz" "$$name"); \
		fi; \
		rm -rf "$$dir"; \
	done; \
	(cd $(DIST) && $(SHA256) git-k_* | sort -k2 > SHA256SUMS)

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
	CGO_ENABLED=0 $(GO) run ./tools/winres

clean:
	rm -f $(BIN)
	rm -rf $(DIST)

releaseLocalLinux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(RELEASE_FLAGS) -ldflags "$(RELEASE_LDFLAGS)" -o $GOGIN/git-k .
