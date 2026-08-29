FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /tcm_relay

##
## Deploy
##
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=UTC

COPY --from=builder /tcm_relay /tcm_relay

# A relative DATABASE_PATH (including the built-in default) resolves
# against the workdir — put it inside the volume so buffered telemetry
# survives a container recreate.
WORKDIR /data
VOLUME /data

ENTRYPOINT ["/tcm_relay"]
