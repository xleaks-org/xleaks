FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /xleaks ./cmd/xleaks/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /xleaks /usr/local/bin/xleaks
EXPOSE 7460 7470
CMD ["xleaks"]
