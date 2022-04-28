# http://www.inanzzz.com/index.php/post/1sfg/multi-stage-docker-build-for-a-golang-application-with-and-without-vendor-directory

# Compile stage
FROM golang:1.16.5 AS go-builder
ENV CGO_ENABLED 0

WORKDIR /device-manager-plugin

ADD . ./

RUN make build

# Final stage
FROM alpine
LABEL org.opencontainers.image.source https://github.com/geoff-coppertop/device-manager-plugin

RUN apk update && apk upgrade && apk add bash

COPY --from=go-builder /device-manager-plugin/out/device-manager-plugin /usr/bin/
COPY config.yml /config/config.yml

# Run
CMD ["/usr/bin/device-manager-plugin"]
