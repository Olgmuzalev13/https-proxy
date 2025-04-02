FROM golang:1.23

ADD go.mod .

COPY . .

RUN go build -o https-proxy main.go
CMD ["./https-proxy"]

EXPOSE 8080 8000