.PHONY: build test clean install

build:
	go build -o protoc-gen-veloce .

test:
	buf generate

install:
	go install .

clean:
	rm -f protoc-gen-veloce
