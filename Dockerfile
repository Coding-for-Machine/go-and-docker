# Golang container
FROM golang:latest


WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN go build -o main ./app/main.go

EXPOSE 3000

CMD ["./main"]
