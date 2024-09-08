FROM node:20-slim as builder-frontend

WORKDIR /build
ADD ./frontend/package.json /build/package.json

RUN yarn install

COPY ./frontend /build

RUN yarn build

FROM golang:1.22.1-alpine as builder-backend

WORKDIR /build
ADD ./go.mod ./go.sum /build/

RUN go mod download

COPY . .
COPY --from=builder-frontend /build/dist ./dist

RUN go build -trimpath -ldflags "-s -w" -o /build/bin/rm-schedule

FROM alpine:3.14

COPY --from=builder-backend /build/bin/rm-schedule /bin/rm-schedule

ENTRYPOINT ["/bin/rm-schedule"]
