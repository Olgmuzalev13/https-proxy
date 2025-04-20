package main

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
)

const (
	proxyAddress = "0.0.0.0:8080"
	infoAddress  = "0.0.0.0:8000"
	caCertPath   = "./certs/ca.crt"
	caKeyPath    = "./certs/ca.key"
	certsPath    = "./certs/ca_host.crt"
)

type InMemoryDB struct {
	RequestAndResponseInDB []RequestAndResponse
}

type RequestAndResponse struct {
	Request  Request
	Response Response
}

type Request struct {
	Method    string
	Path      string
	GetParams map[string]any
	Headers   map[string]string
	Cookie    map[string]any
	Body      string
}

type Response struct {
	Code    int
	Message string
	Headers map[string]string
	Body    string
}

var db = InMemoryDB{
	[]RequestAndResponse{{
		Request: Request{
			Method: "POST",
			Path:   "/path1/path2",
			GetParams: map[string]any{
				"x": 123,
				"y": "qwe",
			},
			Headers: map[string]string{
				"Host":   "example.org",
				"Header": "value",
			},
			Cookie: map[string]any{
				"cookie1": 1,
				"cookie2": "qwe",
			},
			Body: "<html>...",
		},
		Response: Response{
			Code:    200,
			Message: "OK",
			Headers: map[string]string{
				"Server": "nginx/1.14.1",
				"Header": "value",
			},
			Body: "<html>...",
		},
	}},
}

func main() {
	//сервер для раздачи произведенных запросов
	info_server := mux.NewRouter()
	info_server.HandleFunc("/requests", requestsList).Methods("GET")
	info_server.HandleFunc("/requests/{id}", requestByID).Methods("GET")
	info_server.HandleFunc("/repeat/{id}", repeatByID).Methods("GET")
	info_server.HandleFunc("/scan/{id}", scanByID).Methods("GET")

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
		go handleConnection(conn)
	}
}

// универсальная часть соединения считывающая и изменяющая запрос и передающая его нужному обработчику
func handleConnection(clientConn net.Conn) {
	defer clientConn.Close()
	buf := make([]byte, 8000)
	n, err := clientConn.Read(buf)
	if err != nil {
		log.Println("Failed to read from client:", err)
		return
	}

	request := string(buf[:n])
	lines, host, port := cutting(strings.Split(request, "\r\n"))
	log.Println("Host and port", host, port)
	request = strings.Join(lines, "\r\n")
	log.Println(request)
	if len(lines) == 0 {
		log.Println("Invalid request:", lines[0])
		return
	}

	if strings.HasPrefix(lines[0], "CONNECT ") {
		handleHTTPSConnection(clientConn, lines[0])
	} else {
		handleHTTPConnection(clientConn, buf[:n], lines, port)
	}
}

// убирает Proxy-Connection узнает host и port и при необходимости меняет путь на относительный
func cutting(lines []string) ([]string, string, string) {
	var result []string
	port := "80"
	host := ""
	for i, line := range lines {
		if i == 0 {
			prom := strings.Split(line, " ")
			if prom[1][:7] == "http://" {
				prom[1] = prom[1][7:]
			}
			hostPort := strings.Split(prom[1], ":")
			if len(hostPort) != 1 {
				host = hostPort[0]
				port = hostPort[1]
			} else {
				host = prom[1]
				prom[1] = "/"
			}
			result = append(result, strings.Join(prom, " "))
		} else if !strings.HasPrefix(line, "Proxy-Connection:") {
			result = append(result, line)
		}
	}
	return result, host, port
}

// обрабатывает https соединение
func handleHTTPSConnection(clientConn net.Conn, connectRequest string) {
	log.Println("HTTPS connection:", connectRequest)
	fields := strings.Split(connectRequest, " ")
	if len(fields) < 2 {
		log.Println("Malformed CONNECT request")
		return
	}
	addr := fields[1]
	hostPort := strings.Split(addr, ":")
	if len(hostPort) != 2 {
		log.Println("Invalid host/port format")
		return
	}

	_, err := clientConn.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
	if err != nil {
		log.Println("Failed to send 200 OK:", err)
		return
	}

	tlsCert, err := tls.LoadX509KeyPair(caCertPath, caKeyPath)
	if err != nil {
		log.Println("Failed to load cert")
		return
	}

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	tlsClientConn := tls.Server(clientConn, tlsConfig)
	if err := tlsClientConn.Handshake(); err != nil {
		log.Println("TLS handshake failed:", err)
		return
	}
	defer tlsClientConn.Close()

	realServerConn, err := tls.Dial("tcp", addr, &tls.Config{})
	if err != nil {
		log.Println("Failed to connect to real server:", err)
		return
	}
	defer realServerConn.Close()

	go io.Copy(realServerConn, tlsClientConn)
	io.Copy(tlsClientConn, realServerConn)
}

// обрабатывает http соединение
func handleHTTPConnection(clientConn net.Conn, request []byte, lines []string, port string) {
	log.Println("HTTP connection:", lines)
	var host string
	for _, line := range lines {
		if strings.HasPrefix(line, "Host: ") {
			host = strings.TrimSpace(strings.TrimPrefix(line, "Host: "))
			break
		}
	}

	if host == "" {
		log.Println("Failed to parse Host header")
		return
	}

	realServerConn, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		log.Println("Failed to connect to real server:", err)
		return
	}
	defer realServerConn.Close()

	_, err = realServerConn.Write(request)
	if err != nil {
		log.Println("Failed to forward request to server:", err)
		return
	}

	go io.Copy(realServerConn, clientConn)
	io.Copy(clientConn, realServerConn)
}

// handler для вывода списка произведенных запросов
func requestsList(w http.ResponseWriter, r *http.Request) {
	log.Println("requestsList started")

	w.Header().Set("Content-Type", "text/html")
	if err := list_tmpl.Execute(w, db.RequestAndResponseInDB); err != nil {
		http.Error(w, "Render error", http.StatusInternalServerError)
		log.Println("Execute error:", err)
	}
}

// handler для вывода запроса
func requestByID(w http.ResponseWriter, r *http.Request) {
	log.Println("requestByID started")
	vars := mux.Vars(r)
	id := vars["id"]
	err, pair := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
	}

	w.Header().Set("Content-Type", "text/html")
	if err := one_request_tmpl.Execute(w, pair); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// handler для повторной отправки запроса
func repeatByID(w http.ResponseWriter, r *http.Request) {
	log.Println("repeatByID started")
	vars := mux.Vars(r)
	id := vars["id"]
	err, pair := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
	}

	w.Header().Set("Content-Type", "text/html")
	if err := repeat_request_tmpl.Execute(w, pair); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// handler для проверки уязвимости запросов на SQL injection – во всех GET/POST/Сookie/HTTP заголовках
func scanByID(w http.ResponseWriter, r *http.Request) {
	log.Println("scanByID started")
	vars := mux.Vars(r)
	id := vars["id"]

	err, pair := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
	}

	w.Header().Set("Content-Type", "text/html")
	if err := scan_tmpl.Execute(w, pair); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

func get_request_from_DB_by_ID(id string) (error, RequestAndResponse) {
	index := -1
	fmt.Sscanf(id, "%d", &index)
	if index < 0 || index >= len(db.RequestAndResponseInDB) {
		return fmt.Errorf("there is no such id in DB"), RequestAndResponse{}
	}
	return nil, db.RequestAndResponseInDB[index]
}

var one_request_tmpl, list_tmpl, repeat_request_tmpl, scan_tmpl *template.Template

func mustParseTemplate(name string) *template.Template {
	t, err := template.ParseFiles(filepath.Join("templates", name))
	if err != nil {
		log.Fatalf("Error parsing %s: %v", name, err)
	}
	return t
}

func init() {
	one_request_tmpl = mustParseTemplate("one_request.html")
	list_tmpl = mustParseTemplate("requests_list.html")
	repeat_request_tmpl = mustParseTemplate("repeat_request.html")
	scan_tmpl = mustParseTemplate("scan.html")
}
