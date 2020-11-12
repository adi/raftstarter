FROM golang:1.15.2-alpine as build
ENV CGO_ENABLED=0
ENV GO111MODULE on

WORKDIR /app
COPY go.* /app/
RUN apk update && apk upgrade && \
    apk add --no-cache git openssh
RUN go mod download
COPY . .
RUN go build -o /out/go-app .

FROM alpine:3.12.0 as bin

COPY --from=build /out/go-app /bin/go-app
ENTRYPOINT ["/bin/go-app"]
