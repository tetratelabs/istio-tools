TOPDIR := ${PWD}
PROTO_SOURCES = $(wildcard *.proto)
PROTO_GO_SOURCES = $(patsubst %.proto,%.pb.go,$(PROTO_SOURCES))

${PROTO_GO_SOURCES}: ${PROTO_SOURCES}
	@protoc \
		--go_out=plugins=grpc:. \
		--descriptor_set_out=onebig.proto-descriptor --include_imports \
		$?

all: ${PROTO_GO_SOURCES}
	@echo "All protos done"

.PHONY: all

clean:
	@rm -f ${PROTO_GO_SOURCES}
	@echo "Cleaned ${PROTO_GO_SOURCES}"
