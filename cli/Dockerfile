ARG BUILDPLATFORM=linux/amd64

FROM --platform=$BUILDPLATFORM golang:1.24-alpine as get-deps

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

FROM get-deps as build
WORKDIR /app
COPY cluster cluster
COPY utils utils
COPY cli cli

FROM build AS arm64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/k3sd -tags prod -mod=readonly -ldflags "-s -w" ./cli

FROM alpine:3.21.3
ENV PATH=/root/.linkerd2/bin:$PATH

RUN apk add --no-cache ca-certificates step-cli curl openssl
RUN curl --proto '=https' --tlsv1.2 -sSfL https://run.linkerd.io/install | sh

COPY --from=arm64 /out/k3sd /usr/local/bin/k3sd
CMD ["sh"]