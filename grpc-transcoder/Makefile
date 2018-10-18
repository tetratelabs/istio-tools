TEST_DIR := test/v1

build: dep
	@go build -o gen-transcoder main.go

proto:
	@$(MAKE) -C $(TEST_DIR) all

dep:
	@dep ensure

all: proto build
	@echo "Done $@"

clean:
	@$(MAKE) -C $(TEST_DIR) $@
	@rm -f gen-transcoder
	@echo "Done $@"

.PHONY: all
