HUB ?= gcr.io/aardvark-avalanche

default: build

build:
	go build -o pilot-proxy ./cmd

build.static-linux:
	GOOS=linux go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo -o pilot-proxy ./cmd

docker: docker.build

docker.build: build.static-linux
	docker build -t ${HUB}/pilot-proxy -f Dockerfile .

docker.run: docker.build
	docker run ${HUB}/pilot-proxy

docker.push: docker.build
	docker push ${HUB}/pilot-proxy