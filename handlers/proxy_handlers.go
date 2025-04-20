package handlers

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"httpproxy/database"
	"httpproxy/dto"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

const (
	caCertPath = "./certs/ca.crt"
	caKeyPath  = "./certs/ca.key"
	certsPath  = "./certs/ca_host.crt"
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

func HandleHTTPConnection(clientConn net.Conn, requestBytes []byte, lines []string, port string) {
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
	defer clientConn.Close()

	// --- Парсинг запроса ---
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(requestBytes)))
	if err != nil {
		log.Println("Failed to parse request:", err)
		return
	}
	reqBodyBytes, _ := io.ReadAll(req.Body)
	req.Body.Close()

	customReq := dto.Request{
		Method: req.Method,
		Path:   host,
		GetParams: func() map[string]any {
			m := make(map[string]any)
			for k, v := range req.URL.Query() {
				m[k] = v[0]
			}
			return m
		}(),
		Headers: func() map[string]string {
			m := make(map[string]string)
			m["Host"] = host
			for k, v := range req.Header {
				m[k] = strings.Join(v, ", ")
			}
			return m
		}(),
		Cookie: func() map[string]any {
			m := make(map[string]any)
			for _, c := range req.Cookies() {
				m[c.Name] = c.Value
			}
			return m
		}(),
		Body:           string(reqBodyBytes),
		Secure:         0,
		FastRerequests: "",
	}

	// --- Отправка запроса на сервер ---
	_, err = realServerConn.Write(requestBytes)
	if err != nil {
		log.Println("Failed to forward request to server:", err)
		return
	}

	// --- Чтение и парсинг ответа ---
	responseReader := bufio.NewReader(realServerConn)
	resp, err := http.ReadResponse(responseReader, req)
	if err != nil {
		log.Println("Failed to parse response:", err)
		return
	}
	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read response body:", err)
		return
	}
	resp.Body.Close()

	customResp := dto.Response{
		Code:    resp.StatusCode,
		Message: resp.Status,
		Headers: func() map[string]string {
			m := make(map[string]string)
			for k, v := range resp.Header {
				m[k] = strings.Join(v, ", ")
			}
			return m
		}(),
		Body: string(respBodyBytes),
	}

	// --- Отправка ответа клиенту ---
	var responseBuf bytes.Buffer
	resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes)) // чтобы Write не ошибся
	err = resp.Write(&responseBuf)
	if err != nil {
		log.Println("Failed to reassemble full response:", err)
		return
	}
	_, err = clientConn.Write(responseBuf.Bytes())
	if err != nil {
		log.Println("Failed to send response to client:", err)
		return
	}

	// --- Логирование запроса и ответа ---
	log.Println("=== REQUEST ===")
	log.Printf("%+v\n", customReq)
	log.Println("=== RESPONSE ===")
	//log.Printf("%+v\n", customResp)

	database.SaveRequestResponse(dto.RequestAndResponse{
		Request:  customReq,
		Response: customResp,
	})
}

func HandleHTTPSConnection(clientConn net.Conn, connectRequest string) {
	log.Println("HTTPS connection:", connectRequest)

	fields := strings.Split(connectRequest, " ")
	if len(fields) < 2 {
		log.Println("Malformed CONNECT request")
		return
	}
	addr := fields[1]

	_, err := clientConn.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
	if err != nil {
		log.Println("Failed to send 200 OK:", err)
		return
	}

	tlsCert, err := tls.LoadX509KeyPair(caCertPath, caKeyPath)
	if err != nil {
		log.Println("Failed to load cert:", err)
		return
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	tlsClientConn := tls.Server(clientConn, tlsConfig)
	if err := tlsClientConn.Handshake(); err != nil {
		log.Println("TLS handshake failed:", err)
		return
	}
	defer tlsClientConn.Close()

	realServerConn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		log.Println("Failed to connect to real server:", err)
		return
	}
	defer realServerConn.Close()

	// Читаем запрос
	req, err := http.ReadRequest(bufio.NewReader(tlsClientConn))
	if err != nil {
		log.Println("Failed to read HTTPS request:", err)
		return
	}
	reqBody, _ := io.ReadAll(req.Body)
	req.Body.Close()

	// Отправляем запрос
	req.Body = io.NopCloser(bytes.NewReader(reqBody))
	if err := req.Write(realServerConn); err != nil {
		log.Println("Failed to forward HTTPS request:", err)
		return
	}

	// Читаем ответ
	resp, err := http.ReadResponse(bufio.NewReader(realServerConn), req)
	if err != nil {
		log.Println("Failed to read HTTPS response:", err)
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Отправляем клиенту
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	resp.Write(tlsClientConn)

	// Сохраняем данные
	requestStruct := dto.Request{
		Method: req.Method,
		Path:   addr,
		GetParams: func() map[string]any {
			m := make(map[string]any)
			for k, v := range req.URL.Query() {
				if len(v) == 1 {
					m[k] = v[0]
				} else {
					m[k] = v
				}
			}
			return m
		}(),
		Headers: func() map[string]string {
			m := make(map[string]string)
			for k, v := range req.Header {
				m[k] = strings.Join(v, ", ")
			}
			return m
		}(),
		Cookie: func() map[string]any {
			m := make(map[string]any)
			for _, c := range req.Cookies() {
				m[c.Name] = c.Value
			}
			return m
		}(),
		Body:           string(reqBody),
		Secure:         1,
		FastRerequests: "",
	}

	responseStruct := dto.Response{
		Code:    resp.StatusCode,
		Message: resp.Status,
		Headers: func() map[string]string {
			m := make(map[string]string)
			for k, v := range resp.Header {
				m[k] = strings.Join(v, ", ")
			}
			return m
		}(),
		Body: string(respBody),
	}
	// --- Логирование запроса и ответа ---
	log.Println("=== REQUEST ===")
	log.Printf("%+v\n", requestStruct)
	log.Println("=== RESPONSE ===")
	//log.Printf("%+v\n", responseStruct)
	database.SaveRequestResponse(dto.RequestAndResponse{
		Request:  requestStruct,
		Response: responseStruct,
	})
}
