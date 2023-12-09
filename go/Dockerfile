FROM golang:1.21.3-alpine3.18 AS binary-builder
RUN apk update && apk upgrade && apk --update add upx
WORKDIR /builder
COPY go.mod go.sum *.go ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags='-w -s -extldflags "-static"' -a \
  -o app && upx -9 app

FROM gcr.io/distroless/static
WORKDIR /app
COPY --from=binary-builder --chown=nonroot:nonroot /builder/app .
ENTRYPOINT ["./app"]
