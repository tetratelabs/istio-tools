PROTO_DIR := protos

build: dep
	@go build -o gen-transcoder main.go

proto:
	@$(MAKE) -C $(PROTO_DIR) all

dep:
	@dep ensure

all: proto build
	@echo "Done $@"

clean:
	@$(MAKE) -C $(PROTO_DIR) $@
	@rm -f gen-transcoder
	@echo "Done $@"

.PHONY: all
