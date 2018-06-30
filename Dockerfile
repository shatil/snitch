# Requires --build-arg PACKAGE=github.com/shatil/snitch to build.
#
# Mount current directory to /go/bin and when run, this container will build
# your artifact for you and when it finishes running, you'll have artifact(s).
FROM golang:1.10
RUN curl -fsL https://github.com/golang/dep/releases/download/v0.4.1/dep-linux-amd64 -o /usr/local/bin/dep
RUN chmod +x /usr/local/bin/dep
# WORKDIR is $GOPATH, which is "/go".
ARG PACKAGE
COPY . src/${PACKAGE}
WORKDIR src/${PACKAGE}
RUN make dep
RUN make test
CMD make install
