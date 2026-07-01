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

FROM alpine:3.14

WORKDIR /app

COPY --from=builder-frontend /build/dist /app/public
COPY --from=builder-backend /build/bin/rm-schedule /bin/rm-schedule

ENTRYPOINT ["/bin/rm-schedule"]
