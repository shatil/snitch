export PACKAGE ?= github.com/shatil/snitch

# Tests, outputting coverage summary.
test:
	go test -cover -race -coverprofile=coverage.txt -covermode=atomic

# Tests, showing HTML coverage summary.
cover-html: test
	go tool cover -html=coverage.txt

# Ensures dependencies are present.
dep:
	dep ensure

# Builds binary to current working directory.
build:
	for each in $(wildcard cmd/*) ; do \
		go build -ldflags='-s -w' $(PACKAGE)/$$each ; \
	done

# Installs binary file(s) to $GOPATH/bin, which might be ~/go/bin.
install:
	for each in $(wildcard cmd/*) ; do \
		go install -ldflags='-s -w' $(PACKAGE)/$$each ; \
	done

# Builds within a Docker container, producing artifact(s) in current dir.
docker-build:
	docker build --build-arg PACKAGE=$(PACKAGE) --pull --tag $(PACKAGE):latest .
	docker run --rm -it -v "$(shell pwd):/go/bin" $(PACKAGE)

# Clean up artifacts.
clean:
	rm -fvr coverage.out main main.zip
