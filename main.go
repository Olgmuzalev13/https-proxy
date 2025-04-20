package main

import (
	"httpproxy/handlers"
	"log"
	"net"
	"net/http"

	"github.com/gorilla/mux"
)

const (
	proxyAddress = "0.0.0.0:8080"
	infoAddress  = "0.0.0.0:8000"
)

func main() {
	//сервер для раздачи произведенных запросов
	info_server := mux.NewRouter()
	info_server.HandleFunc("/requests", handlers.RequestsList).Methods("GET")
	info_server.HandleFunc("/requests/{id}", handlers.RequestByID).Methods("GET")
	info_server.HandleFunc("/repeat/{id}", handlers.RepeatByID).Methods("GET")
	info_server.HandleFunc("/scan/{id}", handlers.ScanByID).Methods("GET")

	go func() {
		log.Printf("Info server listening on %s", infoAddress)
		err := http.ListenAndServe(infoAddress, info_server)
		if err != nil {
			log.Fatal(err)
		}
	}()
	//сервер для принятия запросов на прокси
	ln, err := net.Listen("tcp", proxyAddress)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", proxyAddress, err)
	}
	defer ln.Close()
	log.Printf("Proxy listening on %s", proxyAddress)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Failed to accept connection:", err)
			continue
		}
		go handlers.HandleConnection(conn)
	}
}
