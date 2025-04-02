FROM golang:1.23

WORKDIR /app

ADD go.mod .

COPY . .

RUN go build -o https-proxy main.go
CMD ["./https-proxy"]

EXPOSE 8080
EXPOSE 8000