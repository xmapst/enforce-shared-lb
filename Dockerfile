FROM golang:latest

WORKDIR /go/src/enforce-shared-lb
COPY * /go/src/enforce-shared-lb

# install upx
RUN sed -i "s/deb.debian.org/mirrors.aliyun.com/g" /etc/apt/sources.list \
  && sed -i "s/security.debian.org/mirrors.aliyun.com/g" /etc/apt/sources.list \
  && apt-get update \
  && apt-get install upx musl-dev git -y

# build code \
RUN go mod tidy \
  && CGO_ENABLED=0 GOOS=linux go build -ldflags \
  "-w -s" cmd/main.go \
  && strip --strip-unneeded main \
  && upx --lzma main

FROM alpine:latest

WORKDIR /app
COPY --from=0 --chmod=a+x /go/src/enforce-shared-lb/main .

ENTRYPOINT ["/app/main"]