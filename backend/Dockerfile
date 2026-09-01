# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && mkdir -p /out/data/audio /out/data/video /out/data/voice-enrollment

FROM alpine:3.22
RUN apk add --no-cache ca-certificates ffmpeg \
    && addgroup -S app \
    && adduser -S -G app app
COPY --from=build /out/server /server
COPY --from=build --chown=app:app /out/data /data
ENV APP_VOICE_STORAGE_DIR=/data/audio
ENV APP_VOICE_ENROLLMENT_STORAGE_DIR=/data/voice-enrollment
ENV APP_VIDEO_STORAGE_DIR=/data/video
EXPOSE 8080
USER app:app
ENTRYPOINT ["/server"]
