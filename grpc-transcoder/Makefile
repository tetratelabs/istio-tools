default: build

dep:
	@dep ensure

build: dep
	@go build -o gen-envoyfilter main.go

clean:
	@rm -f gen-envoyfilter
