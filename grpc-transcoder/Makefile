## Copyright 2018 Tetrate.io, Inc.
##
## Licensed under the Apache License, Version 2.0 (the "License");
## you may not use this file except in compliance with the License.
## You may obtain a copy of the License at
##
##     http://www.apache.org/licenses/LICENSE-2.0
##
## Unless required by applicable law or agreed to in writing, software
## distributed under the License is distributed on an "AS IS" BASIS,
## WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
## See the License for the specific language governing permissions and
## limitations under the License.

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
