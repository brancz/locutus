ARG GOVERSION=1.15.7
FROM golang:${GOVERSION} as builder
ARG GOARCH
ENV GOARCH=${GOARCH}
WORKDIR /src/locutus
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . /src/locutus

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/locutus

FROM docker.io/alpine:3.13.0
COPY --from=builder /go/bin/locutus /

USER nobody

ENTRYPOINT ["/locutus"]
