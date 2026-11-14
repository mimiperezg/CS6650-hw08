FROM --platform=linux/amd64 golang:1.27.1 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /dynamo-service .

FROM --platform=linux/amd64 alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /dynamo-service /dynamo-service
EXPOSE 8080
ENTRYPOINT ["/dynamo-service"]
