# Snitch
Package `github.com/shatil/snitch` collects and (optionally) publishes ECS
Cluster capacity metrics.

[//]: # (Thank you, aws-lambda-go, for the badges.)
[![Documentation][1]][2]
[![Build Status][5]][6]
[![Go Report Card][3]][4]
[![Code Coverage][7]][8]

[1]: https://godoc.org/github.com/shatil/snitch?status.svg
[2]: https://godoc.org/github.com/shatil/snitch
[3]: https://goreportcard.com/badge/github.com/shatil/snitch
[4]: https://goreportcard.com/report/github.com/shatil/snitch
[5]: https://travis-ci.org/shatil/snitch.svg?branch=master
[6]: https://travis-ci.org/shatil/snitch
[7]: https://codecov.io/gh/shatil/snitch/branch/master/graph/badge.svg
[8]: https://codecov.io/gh/shatil/snitch

AWS SDK requires you to specify the AWS Region you wish to interact with,
which you can do at runtime with environment variable `AWS_REGION`.

# Development
`git clone` into your `$GOPATH/src/`, which may be `~/go/src/`.

```bash
git clone \
    git@github.com:shatil/snitch.git \
    $GOPATH/src/github.com/shatil/snitch
```

For deployment-worth artifacts built locally, I recommend `make docker-build`.
Summary of interesting `Makefile` targets:

## Dependencies
`make dep` will fetch dependencies to `vendor/`.

## Test
`make test` runs all tests and `make cover-html` will do that _and_ generate
HTML code coverage.

## Run
You can build and run the binaries or pick a binary and:

```bash
AWS_REGION=ca-central-1 go run cmd/snitch/main.go
```

## Build
`make build` builds all binaries in `cmd/` and deposits them in this
folder.

## Install
`make install` installs the binaries to `$GOPATH/bin/`. You probably don't
love this repo enough to do that--it's there mostly to simplify compiling
this repository using Docker.
