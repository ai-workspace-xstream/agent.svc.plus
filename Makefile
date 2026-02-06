 BINARY_NAME=agent-svc-plus
INSTALL_BIN=/usr/local/bin/$(BINARY_NAME)
SERVICE_NAME=agent-svc-plus

.PHONY: all build install upgrade

all: build

export PATH := $(PATH):/usr/local/go/bin

build:
	/usr/local/go/bin/go mod tidy
	/usr/local/go/bin/go build -o $(BINARY_NAME) ./cmd/agent

install: build
	install -m 755 $(BINARY_NAME) $(INSTALL_BIN)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_BIN)"

upgrade: install
	systemctl restart $(SERVICE_NAME)
	@echo "Restarted $(SERVICE_NAME)"
