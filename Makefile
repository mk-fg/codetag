PROJECT=codetag

SHELL=/bin/sh

SRC_DIR=src/$(PROJECT)
BIN_DIR=bin
BIN_FILE=$(PROJECT)

export GOPATH=$(PWD)

all: | clean compile test

clean:
	go clean $(PROJECT)

compile:
	@grep -q filter=golang .gitattributes &&\
		find src -type f -name '*.go' -exec golang_filter curl '{}' \;
	go build -o $(BIN_FILE) $(PROJECT);\
		status=$$?;\
		grep -q filter=golang .gitattributes &&\
			find src -type f -name '*.go' -exec golang_filter uncurl '{}' \;;\
		exit $$status
	@mkdir -p $(BIN_DIR)
	mv $(BIN_FILE) $(BIN_DIR)

test: compile
	go test $(PROJECT)/...
