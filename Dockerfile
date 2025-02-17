# Golang container
FROM golang:1.23


WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN go build -o main ./app/main2.go

EXPOSE 3000

CMD ["./main"]
