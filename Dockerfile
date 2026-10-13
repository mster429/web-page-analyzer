# build
FROM golang:1.27-alpine AS build
WORKDIR /src

# copy the dependency list
COPY go.mod go.sum ./
RUN go mod download

# copy source code and compile
COPY . .
RUN CGO_ENABLED=0 go build -o /out/analyzer ./cmd/server

#run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D app
USER app
COPY --from=build /out/analyzer /analyzer
EXPOSE 8080
ENTRYPOINT ["/analyzer"]