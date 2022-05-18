FROM golang:1.18-alpine as build

COPY . /usr/src/code

WORKDIR /usr/src/code

RUN go build


FROM alpine:latest as production-build

RUN apk add --update --no-cache supervisor gd && rm -rf /var/cache/apk/*

RUN mkdir /opt/pengine
COPY --from=build /usr/src/code/media-manager /opt/media-manager/media-manager
COPY --from=build /usr/src/code/fonts /opt/media-manager/fonts


ADD docker_assets/supervisord.conf /etc/supervisord.conf

# This command runs your application, comment out this line to compile only
CMD ["/usr/bin/supervisord","-n", "-c", "/etc/supervisord.conf"]

LABEL Name=pengine Version=0.0.1

