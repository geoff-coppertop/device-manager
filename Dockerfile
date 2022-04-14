# http://www.inanzzz.com/index.php/post/1sfg/multi-stage-docker-build-for-a-golang-application-with-and-without-vendor-directory

# Compile stage
FROM golang:1.16.5 AS go-builder
ENV CGO_ENABLED 0

WORKDIR /device-manager-plugin

ADD . ./

RUN make build

# Final stage
FROM scratch
LABEL org.opencontainers.image.source https://github.com/geoff-coppertop/device-manager-plugin

COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go-builder /device-manager-plugin/out/device-manager-plugin /
COPY config.yml /config/config.yml

# Run
CMD ["/device-manager-plugin"]
