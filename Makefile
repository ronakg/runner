UT_COV=$(PWD)/cov.out

GOCMD=go
GOBUILD=$(GOCMD) build -race -v -trimpath
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test -race -timeout 2m -v -count=1 -coverprofile=$(UT_COV) ./...
GOCOVER=$(GOCMD) tool cover
GOFMT=gofmt -w
GOVET=go vet

format:
	@echo ">>>> Formatting code..."
	$(GOFMT) .
	@echo "<<<< Done formatting code!"
	@# Help: Auto-format source code

lint: format
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

container:
	@echo ">>>> Running container build and install..."
	mkdir -p /tmp/runner
	cd pkg/lib/container && \
		$(GOBUILD) && \
		cp container /tmp/runner/
	@echo "<<<< Done installing container!"
	@# Help: Build and install container


test: lint protogen container
	@echo ">>>> Testing $(PWD)..."
	$(GOTEST)
	@echo "<<<< Done testing!"
	@# Help: Run all the unit tests

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
