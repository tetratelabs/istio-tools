HUB ?= gcr.io/aardvark-avalanche

default: build

build:
	go build -o grpc-transcoder-proxy ./cmd

build.static-linux:
	GOOS=linux go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o grpc-transcoder-proxy ./cmd

docker: docker.build

docker.build: build.static-linux
	docker build -t ${HUB}/grpc-transcoder-proxy -f Dockerfile .

docker.run: docker.build
	docker run ${HUB}/grpc-transcoder-proxy

docker.push: docker.build
	docker push ${HUB}/grpc-transcoder-proxy