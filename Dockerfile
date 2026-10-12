FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /portal ./cmd/portal

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /portal /app/portal
ENV PORTAL_DATA_DIR=/app/data TZ=Asia/Jakarta
VOLUME /app/data
EXPOSE 8090 8091
CMD ["/app/portal"]
