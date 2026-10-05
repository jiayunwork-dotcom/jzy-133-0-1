# 后端构建：Go 1.22 alpine 多阶段
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 spc
COPY --from=build /out/server /usr/local/bin/server
USER spc
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
