package handlers

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"strings"
)

const (
	caCertPath   = "./certs/ca.crt"
	caKeyPath    = "./certs/ca.key"
	certsPath    = "./certs/ca_host.crt"
)

// универсальная часть соединения считывающая и изменяющая запрос и передающая его нужному обработчику
func HandleConnection(clientConn net.Conn) {
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
		HandleHTTPSConnection(clientConn, lines[0])
	} else {
		HandleHTTPConnection(clientConn, buf[:n], lines, port)
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
func HandleHTTPSConnection(clientConn net.Conn, connectRequest string) {
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
func HandleHTTPConnection(clientConn net.Conn, request []byte, lines []string, port string) {
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
