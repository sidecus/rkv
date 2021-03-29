#
# build stage
#
FROM golang:alpine as build
WORKDIR /app

# Set go proxy...:(
RUN go env -w GO111MODULE=on && \
    go env -w GOPROXY=https://mirrors.aliyun.com/goproxy/,direct

# Copy go.mod and go.sum first and download dependencies for better caching layer
COPY go.mod go.sum ./
RUN go mod download

# Copy all other files
COPY . .

# Build server
RUN go build ./cmd/rkv

#
# Imaging stage
#
FROM alpine:latest
WORKDIR /app
COPY --from=build /app/rkv .

ENTRYPOINT ["/app/rkv"]
CMD ["--help"]