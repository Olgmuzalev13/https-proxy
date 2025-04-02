package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

const (
	proxyAddress = "127.0.0.1:8089"
	infoAddress  = "127.0.0.1:8000"
	caCertPath   = "./certs/ca.crt"
	caKeyPath    = "./certs/ca.key"
	certsPath    = "./certs/ca_host.crt"
)

func requestsList(w http.ResponseWriter, r *http.Request) {
	log.Println("RequestsList started")
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Requests List</title>
			<meta charset="UTF-8">
		</head>
		<body>
			<h1>Список запросов</h1>
			<table border="1">
				<tr>
					<th>ID</th>
					<th>Метод</th>
					<th>Путь</th>
					<th>Параметры</th>
					<th>Заголовки</th>
					<th>Cookie</th>
					<th>Тело</th>
				</tr>`
	for i, req := range db.RequestInDB {
		htmlContent += fmt.Sprintf(`
			<tr>
				<td><a href="/requests/%d">%d</a></td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
			</tr>`, i, i, req.Method, req.Path, req.GetParams, req.Headers, req.Cookie, req.Body)
	}
	htmlContent += `
			</table>
		</body>
		</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func requestByID(w http.ResponseWriter, r *http.Request) {
	log.Println("requestByID started")
	vars := mux.Vars(r)
	id := vars["id"]
	index := -1

	fmt.Sscanf(id, "%d", &index)
	if index < 0 || index >= len(db.RequestInDB) {
		http.Error(w, "Invalid ID", http.StatusNotFound)
		return
	}

	req := db.RequestInDB[index]
	resp := db.ResponseInDB[index]

	htmlContent := fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>Request Details</title>
			<meta charset="UTF-8">
		</head>
		<body>
			<h1>Запрос</h1>
			<table border="1">
				<tr><th>Метод</th><td>%s</td></tr>
				<tr><th>Путь</th><td>%s</td></tr>
				<tr><th>Параметры</th><td>%s</td></tr>
				<tr><th>Заголовки</th><td>%s</td></tr>
				<tr><th>Cookie</th><td>%s</td></tr>
				<tr><th>Тело</th><td>%s</td></tr>
			</table>
			<h2>Ответ</h2>
			<table border="1">
				<tr><th>Статус</th><td>%s</td></tr>
				<tr><th>Заголовки</th><td>%s</td></tr>
				<tr><th>Тело</th><td>%s</td></tr>
			</table>
		</body>
		</html>`,
		req.Method, req.Path, req.GetParams, req.Headers, req.Cookie, req.Body,
		resp.Method, resp.Headers, resp.Body)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

type InMemoryDB struct {
	RequestInDB []Request
	ResponseInDB []Request
}

type Request struct {
	Method string
	Path string
	GetParams string
	Headers string
	Cookie string
	Body string
}

var db  = InMemoryDB{}

func main() {
	info_server := mux.NewRouter()
	info_server.HandleFunc("/requests", requestsList).Methods("GET")
	info_server.HandleFunc("/requests/{id}", requestByID).Methods("GET")

	go func() {
		log.Printf("Info server listening on %s", infoAddress)
		err := http.ListenAndServe(infoAddress, info_server)
		if err != nil {
			log.Fatal(err)
		}
	}()

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
		handleHTTPConnection(clientConn, buf[:n], lines)
	}
}

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

func handleHTTPConnection(clientConn net.Conn, request []byte, lines []string) {
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

	realServerConn, err := net.Dial("tcp", host+":80")
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

	// Записываем запрос в БД
	db.RequestInDB = append(db.RequestInDB, Request{
		Method:  lines[0],
		Path:    host,
		Headers: strings.Join(lines, "\n"),
		Body:    string(request),
	})

	responseBuf, err := io.ReadAll(realServerConn)
	if err != nil {
		log.Println("Failed to read response from server:", err)
		return
	}

	// Записываем ответ в БД
	db.ResponseInDB = append(db.ResponseInDB, Request{
		Method: "HTTP Response",
		Body:   string(responseBuf),
	})

	go io.Copy(realServerConn, clientConn)
	io.Copy(clientConn, realServerConn)
}
