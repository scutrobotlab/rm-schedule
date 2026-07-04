FROM node:20-slim as builder-frontend

WORKDIR /build
ADD ./frontend/package.json /build/package.json

RUN yarn install

COPY ./frontend /build

RUN yarn build

FROM golang:1.26.4-alpine as builder-backend

WORKDIR /build
ADD ./go.mod ./go.sum /build/

RUN go mod download

COPY . .

RUN go build -trimpath -ldflags "-s -w" -o /build/bin/rm-schedule

FROM alpine:3.20

RUN apk add --no-cache chromium ca-certificates font-noto-cjk ttf-freefont

# 关闭 Iris 的 debug 日志（iris.Default() 默认 debug 级别）
ENV SCHEDULE_LOG_LEVEL=info

WORKDIR /app

COPY --from=builder-frontend /build/dist /app/public
COPY --from=builder-backend /build/bin/rm-schedule /bin/rm-schedule

ENTRYPOINT ["/bin/rm-schedule"]
