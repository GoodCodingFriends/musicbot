FROM golang:alpine as build-env
RUN apk --update add git
ADD . /src
ENV CGO_ENABLED=0
RUN cd /src && go build -o musicbot

FROM alpine
WORKDIR /app
RUN apk --update add ca-certificates youtube-dl ffmpeg
COPY --from=build-env /src/musicbot /app/
ENTRYPOINT ["./musicbot"]
