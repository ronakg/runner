UT_COV=$(PWD)/cov.out

GOCMD=go
GOBUILD=$(GOCMD) build -race -v -trimpath
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test -race -timeout 2m -v -count=1 -coverprofile=$(UT_COV)
GOCOVER=$(GOCMD) tool cover
GOFMT=gofmt -w
GOVET=go vet
RUNNER_HOME=/tmp/runner
CERTS_DIR=$(RUNNER_HOME)/certs

OS := $(shell uname -s)
ifeq ($(OS),Darwin)
	# https://github.com/golang/go/issues/49138#issuecomment-951401558
	export MallocNanoZone=0
endif

CMDS=client server

build: $(CMDS:%=%.bin)
	@# Help: Build binaries for client and server

install: build
	@echo ">>>> Installing client and server binaries to /tmp/runner/bin"
	cp -r $(PWD)/bin $(RUNNER_HOME)
	@echo "<<<< Done installing binaries!"
	@# Help: Install client and server binaries

format:
	@echo ">>>> Formatting code..."
	$(GOFMT) .
	@echo "<<<< Done formatting code!"
	@# Help: Auto-format source code

lint: protogen format
	@echo ">>>> Running static analysis..."
	$(GOVET) ./...
	@echo "<<<< Done running static analysis!"
	@# Help: Run static analysis

protogen:
	@echo ">>>> Running protobuf bindings generation..."
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		pkg/proto/*.proto
	@echo "<<<< Done running protobuf bindings generation!"
	@# Help: Generate protobuf bindings

rootfs:
	@echo ">>>> Setting up root filesystem..."
	rm -rf $(RUNNER_HOME)/rootfs && \
		mkdir -p $(RUNNER_HOME)/rootfs && \
		tar -xzf resources/alpine-minirootfs-3.15.0-x86_64.tar.gz -C $(RUNNER_HOME)/rootfs/
	@echo "<<<< Done settings up root filesystem!"
	@# Help: Set up root filesystem

certs:
	@echo ">>>> Generating certificates..."
	mkdir -p $(RUNNER_HOME)/certs
	cp -r resources/certs $(RUNNER_HOME)/
	@echo "<<<< Done generating certificates!"
	@# Help: Generate certificates

$(CMDS:%=%.bin): %.bin: format lint protogen rootfs
	@echo ">>>> Building $* binary..."
	mkdir -p $(PWD)/bin
	cd $(PWD)/cmd/$* && \
		$(GOBUILD) -o $(PWD)/bin/$(*F)
	@echo "<<<< Done building $* binary!"
	@# Help: Build binary for $*

clean:
	@echo ">>>> Cleaning everything up..."
	rm -rf $(RUNNER_HOME)
	@echo "<<<< Done cleaning up!"
	@# Help: Clean everything

test: lint protogen rootfs certs install
	@echo ">>>> Running tests in $(PWD)..."
	$(GOTEST) ./...
	@echo "<<<< Done testing!"
	@# Help: Run all the tests

integration.test: lint protogen rootfs certs install
	@echo ">>>> Running integration tests..."
	$(GOTEST) $(PWD)/integration
	@echo "<<<< Done running integration tests!"
	@# Help: Run integration tests

coverage:
	$(GOCOVER) -html=$(UT_COV)
	@# Help: Show unit test coverage in browser

help:
	@printf "%-20s %s\n" "Target" "Description"
	@printf "%-20s %s\n" "------" "-----------"
	@make -pqR : 2>/dev/null \
		| awk -v RS= -F: '/^# File/,/^# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' \
		| sort \
		| egrep -v -e '^[^[:alnum:]]' -e '^$@$$' \
		| xargs -I _ sh -c 'printf "%-20s " _; make _ -nB | (grep -i "^# Help:" || echo "") | tail -1 | sed "s/^# Help: //g"'
