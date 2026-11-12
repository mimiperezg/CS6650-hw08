FROM golang:1.27.1

WORKDIR /app

# Copy our module files into the directory
COPY ./go.mod ./go.sum ./

# Download all of our necessary packages
RUN go mod download

# Copy all of the .go code from our sub folder into the root
COPY ./*.go ./

# Build our code and output the executable go code into /docker-product-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /docker-product-service

# Expose our port
EXPOSE 8080

# Run the executable
CMD ["/docker-product-service"]