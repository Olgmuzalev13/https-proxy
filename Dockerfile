FROM golang:1.23

WORKDIR /app

#ADD go.mod .

#COPY . .
COPY main.go .

RUN go build -o https-proxy main.go
CMD ["./https-proxy"]
#WORKDIR /

#RUN go run *.go

#FROM alpine

EXPOSE 8080