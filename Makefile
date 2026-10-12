# Toolchain protobuf (tarea 3_5_1).
# `make tools` deja buf + plugins en ./bin (sin exigir Go local: los plugins
# se compilan en docker). `make generate` corre buf con ./bin en el PATH.

BIN_DIR := $(CURDIR)/bin
BUF_VERSION := v1.73.0
PROTOC_GEN_GO_VERSION := v1.36.12
PROTOC_GEN_GO_GRPC_VERSION := v1.6.2
GO_IMAGE := golang:1.26

.PHONY: tools generate clean-tools

tools: $(BIN_DIR)/buf $(BIN_DIR)/protoc-gen-go $(BIN_DIR)/protoc-gen-go-grpc

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(BIN_DIR)/buf: | $(BIN_DIR)
	curl -sSL https://github.com/bufbuild/buf/releases/download/$(BUF_VERSION)/buf-Linux-x86_64.tar.gz -o /tmp/buf.tar.gz
	tar -xzf /tmp/buf.tar.gz -C /tmp
	install -m 0755 /tmp/buf/bin/buf $(BIN_DIR)/buf
	rm -rf /tmp/buf.tar.gz /tmp/buf

$(BIN_DIR)/protoc-gen-go $(BIN_DIR)/protoc-gen-go-grpc: | $(BIN_DIR)
	docker run --rm -v $(BIN_DIR):/out $(GO_IMAGE) \
		sh -c "GOBIN=/out go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION) && GOBIN=/out go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)"

generate: tools
	cd back/proto && PATH="$(BIN_DIR):$$PATH" buf generate

clean-tools:
	rm -rf $(BIN_DIR)
