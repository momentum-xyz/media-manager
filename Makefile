all: build

build:
	go build -o ./bin/renderman

run: build
	./bin/renderman

.PHONY: build run
