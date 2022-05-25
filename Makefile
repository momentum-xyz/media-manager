all: build

build:
	go build -o ./bin/media-manager

run: build
	./bin/media-manager

.PHONY: build run
