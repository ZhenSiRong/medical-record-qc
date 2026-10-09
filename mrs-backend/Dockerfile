# 多阶段构建：产出仅含二进制的小镜像
FROM golang:1-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /out/mrs .

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
WORKDIR /app
COPY --from=build /out/mrs .
COPY config.yaml .
ENV TZ=Asia/Shanghai
EXPOSE 8080
ENTRYPOINT ["/app/mrs"]
