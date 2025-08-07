# Makefile para o projeto go-filewatcher


APP_NAME=gfw
BIN_DIR=bin
BIN=$(BIN_DIR)/$(APP_NAME)

.PHONY: all build clean run

all: build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) main.go

run: build
	./$(BIN)

clean:
	rm -rf $(BIN_DIR)
