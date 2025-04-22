package handlers

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"html/template"
	"httpproxy/database"
	"httpproxy/dto"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// handler для вывода списка произведенных запросов
func RequestsList(w http.ResponseWriter, r *http.Request) {
	log.Println("requestsList started")

	w.Header().Set("Content-Type", "text/html")
	all_requests, err := database.GetAllRequests()
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		log.Println("Execute error:", err)
	}
	if err := list_tmpl.Execute(w, all_requests); err != nil {
		http.Error(w, "Render error", http.StatusInternalServerError)
		log.Println("Execute error:", err)
	}
}

// handler для вывода запроса
func RequestByID(w http.ResponseWriter, r *http.Request) {
	log.Println("requestByID started")
	vars := mux.Vars(r)
	id := vars["id"]
	pair, err := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
	}

	w.Header().Set("Content-Type", "text/html")
	if err := one_request_tmpl.Execute(w, pair); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

func RepeatByID(w http.ResponseWriter, r *http.Request) {
	log.Println("repeatByID started")

	vars := mux.Vars(r)
	id := vars["id"]

	pair, err := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
		return
	}
	var repeatResp dto.Response
	if pair.Request.Secure == 0 {
		repeatResp, err = HTTPrerequest(pair)
	} else {
		repeatResp, err = HTTPSrerequest(pair)
	}
	if err != nil {
		log.Panicln("failed to rerequest", err.Error())
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	newData := dto.Rerequested{Old: pair, NewResponse: repeatResp}
	w.Header().Set("Content-Type", "text/html")
	if err := repeat_request_tmpl.Execute(w, newData); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}
func HTTPrerequest(pair dto.RequestAndResponse) (dto.Response, error) {
	log.Println("HTTPrerequest started")
	original := cloneRequest(pair.Request)
	log.Println("original.Path:", original.Path)
	log.Println("original.GetParams:", original.GetParams)
	// Формируем базовый URL с GET параметрами
	u := url.URL{
		Scheme: "http",
		Host:   original.Headers["Host"],
		Path:   original.Path,
	}
	//if len(original.GetParams) > 0 && !strings.Contains(original.Path, "?") {
	if len(original.GetParams) > 0 {
		if strings.Contains(original.Path, "?"){
			u.Path = original.Path[:strings.Index(original.Path, "?")]
		}
		query := url.Values{}
		for k, v := range original.GetParams {
			query.Set(k, fmt.Sprint(v))
		}
		u.RawQuery = query.Encode()
	}

	// Тело запроса (POST/PUT)
	var bodyReader io.Reader
	if original.Method == "POST" || original.Method == "PUT" {
		bodyReader = strings.NewReader(original.Body)
	}

	req, err := http.NewRequest(original.Method, u.String(), bodyReader)
	if err != nil {
		return dto.Response{}, fmt.Errorf("failed to build request: %v", err)
	}

	// Заголовки
	for k, v := range original.Headers {
		if k != "Host" {
			req.Header.Set(k, v)
		}
	}
	req.Host = original.Headers["Host"]

	// Куки
	for name, val := range original.Cookie {
		req.AddCookie(&http.Cookie{
			Name:  name,
			Value: fmt.Sprint(val),
		})
	}

	// Логирование финального запроса
	//log.Printf("http requesting - %v %v\nHeaders: %v\nCookies: %v\nBody: %v\n", req.Method, req.URL.String(), req.Header, req.Cookies(), original.Body)
	log.Println("rerequesting - ", req)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return dto.Response{}, fmt.Errorf("failed to repeat request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return dto.Response{}, fmt.Errorf("failed to read response body: %v", err)
	}

	repeatResp := dto.Response{
		Code:    resp.StatusCode,
		Message: resp.Status,
		Headers: func() map[string]string {
			m := make(map[string]string)
			for k, v := range resp.Header {
				m[k] = strings.Join(v, ", ")
			}
			return m
		}(),
		Body: string(bodyBytes),
	}
	log.Println("response - ", repeatResp)
	return repeatResp, nil
}
// func HTTPrerequest(pair dto.RequestAndResponse) (dto.Response, error) {
// 	log.Println("HTTPrerequest started")
// 	original := cloneRequest(pair.Request)
// 	//log.Println("saved - ", original)
// 	targetURL := "http://" + original.Headers["Host"] + original.Path

// 	req, err := http.NewRequest(original.Method, targetURL, strings.NewReader(original.Body))
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to build request: %v", err)
// 	}

// 	// Установка заголовков (кроме Host)
// 	for k, v := range original.Headers {
// 		if k != "Host" {
// 			req.Header.Set(k, v)
// 		}
// 	}
// 	req.Host = original.Headers["Host"]

// 	for name, val := range original.Cookie {
// 		req.AddCookie(&http.Cookie{Name: name, Value: fmt.Sprint(val)})
// 	}

// 	client := &http.Client{
// 		CheckRedirect: func(req *http.Request, via []*http.Request) error {
// 			// Отклоняем редирект, возвращая специальную ошибку
// 			return http.ErrUseLastResponse
// 		},
// 	}
// 	log.Println("http requesting - ", req)
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to repeat request: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	bodyBytes, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to read response body: %v", err)
// 	}

// 	repeatResp := dto.Response{
// 		Code:    resp.StatusCode,
// 		Message: resp.Status,
// 		Headers: func() map[string]string {
// 			m := make(map[string]string)
// 			for k, v := range resp.Header {
// 				m[k] = strings.Join(v, ", ")
// 			}
// 			return m
// 		}(),
// 		Body: string(bodyBytes),
// 	}
// 	return repeatResp, nil
// }

func HTTPSrerequest(pair dto.RequestAndResponse) (dto.Response, error) {
	log.Println("HTTPSrerequest started")
	saved := cloneRequest(pair.Request)
	log.Println("saved", saved, saved.Path)

	parts := strings.SplitN(saved.Path, "/", 2)
	if len(parts) != 2 {
		return dto.Response{}, fmt.Errorf("invalid saved.Path format: %s", saved.Path)
	}
	host := parts[0]
	path := "/" + parts[1]
	address := host + ":443"

	// Подготовим тело
	bodyReader := io.NopCloser(strings.NewReader(saved.Body))

	// Правильная сборка запроса
	req := &http.Request{
		Method:     saved.Method,
		URL:        &url.URL{Path: path},
		Host:       host,
		Header:     make(http.Header),
		Body:       bodyReader,
		RequestURI: path,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}

	for k, v := range saved.Headers {
		req.Header.Set(k, v)
	}

	if len(saved.Cookie) > 0 {
		var cookies []string
		for k, v := range saved.Cookie {
			cookies = append(cookies, fmt.Sprintf("%s=%v", k, v))
		}
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}

	log.Println("resending request - ", req)

	// TLS-соединение
	conn, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return dto.Response{}, fmt.Errorf("tls dial error: %v", err)
	}
	defer conn.Close()

	if err := req.Write(conn); err != nil {
		return dto.Response{}, fmt.Errorf("failed to write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return dto.Response{}, fmt.Errorf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return dto.Response{}, fmt.Errorf("failed to read response body: %v", err)
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

	return responseStruct, nil
}

// handler для проверки уязвимости запросов на SQL injection – во всех GET/POST/Сookie/HTTP заголовках
func ScanByID(w http.ResponseWriter, r *http.Request) {
	log.Println("scanByID started")
	vars := mux.Vars(r)
	id := vars["id"]

	pair, err := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
		return
	}
	log.Println("request to check - ", pair.Request)
	originalResp, err := HTTPrerequest(pair)
	if err != nil {
		return
	}
	originalLength := getContentLength(originalResp)
	originalCode := originalResp.Code

	injections := []string{`'`, `"`}

	vulnerableParams := []string{}
	var injectionResults []dto.InjectionInfo

	original := cloneRequest(pair.Request)

	check := func(paramType, key, originalValue, payload string, mutate func(string) dto.Request) {
		modified := mutate(payload)
		resp, err := HTTPrerequest(dto.RequestAndResponse{Request: modified})
		if err != nil {
			return
		}
		contentLength := getContentLength(resp)
		safe := resp.Code == originalCode && contentLength == originalLength
		if !safe {
			vulnerableParams = append(vulnerableParams, fmt.Sprintf("%s param %s", paramType, key))
		}
		injectionResults = append(injectionResults, dto.InjectionInfo{
			Safe:          safe,
			Code:          resp.Code,
			ContentLength: contentLength,
			Injections:    template.HTML(fmt.Sprintf("%s param %s → %s", paramType, key, payload)),
		})
	}

	// GET
	for key, val := range original.GetParams {
		for _, inj := range injections {
			strVal := fmt.Sprint(val)
			check("GET", key, strVal, strVal+inj, func(payload string) dto.Request {
				modified := cloneRequest(original)
				modified.GetParams[key] = payload
				return modified
			})
		}
	}

	// POST
	if original.Method == "POST" && original.Body != "" {
		for _, inj := range injections {
			check("POST", "body", original.Body, original.Body+inj, func(payload string) dto.Request {
				modified := cloneRequest(original)
				modified.Body = payload
				return modified
			})
		}
	}

	// Cookies
	for key, val := range original.Cookie {
		for _, inj := range injections {
			strVal := fmt.Sprint(val)
			check("Cookie", key, strVal, strVal+inj, func(payload string) dto.Request {
				modified := cloneRequest(original)
				modified.Cookie[key] = payload
				return modified
			})
		}
	}

	// Headers
	for key, val := range original.Headers {
		for _, inj := range injections {
			check("Header", key, val, val+inj, func(payload string) dto.Request {
				modified := cloneRequest(original)
				modified.Headers[key] = payload
				return modified
			})
		}
	}

	safety_info := "SQL injection – уязвимых параметров не найдено"
	safety_status := true
	if len(vulnerableParams) > 0 {
		safety_info = "SQL injection – уязвимые параметры: " + strings.Join(vulnerableParams, ", ")
		safety_status = false
	}

	full_data := dto.Scanned{
		Info:           pair,
		SecurityInfo:   safety_info,
		Safe:           safety_status,
		InjectionsList: injectionResults,
	}
	log.Println("end - ", pair.Request)
	w.Header().Set("Content-Type", "text/html")
	if err := scan_tmpl.Execute(w, full_data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
func cloneMapstr(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
func cloneRequest(req dto.Request) dto.Request {
	return dto.Request{
		ID:        req.ID,
		Method:    req.Method,
		Path:      req.Path,
		GetParams: cloneMap(req.GetParams),
		Headers:   cloneMapstr(req.Headers),
		Cookie:    cloneMap(req.Cookie),
		Body:      req.Body,
	}
}
// func ScanByID(w http.ResponseWriter, r *http.Request) {
// 	log.Println("scanByID started")
// 	vars := mux.Vars(r)
// 	id := vars["id"]

// 	pair, err := get_request_from_DB_by_ID(id)
// 	if err != nil {
// 		http.Error(w, "Invalid ID", http.StatusNotFound)
// 		return
// 	}

// 	originalResp, err := HTTPrerequest(pair)
// 	if err != nil {
// 		return
// 	}
// 	originalLength := getContentLength(originalResp)
// 	originalCode := originalResp.Code

// 	injections := []string{`'`, `"`}

// 	vulnerableParams := []string{}
// 	triedInjections := []template.HTML{}

// 	check := func(paramType, key, value, payload string, mutate func(string) dto.Request) {
// 		triedInjections = append(triedInjections, template.HTML(fmt.Sprintf("%s param '%s' → %s", paramType, key, value+payload)))
// 		modified := mutate(payload)
// 		resp, err := HTTPrerequest(dto.RequestAndResponse{Request: modified})
// 		if err != nil {
// 			return
// 		}
// 		if resp.Code != originalCode || getContentLength(resp) != originalLength {
// 			vulnerableParams = append(vulnerableParams, fmt.Sprintf("%s param '%s'", paramType, key))
// 		}
// 	}

// 	original := pair.Request

// 	// GET
// 	for key, val := range original.GetParams {
// 		for _, inj := range injections {
// 			strVal := fmt.Sprint(val)
// 			check("GET", key, strVal, strVal+inj, func(payload string) dto.Request {
// 				modified := original
// 				modified.GetParams[key] = payload
// 				return modified
// 			})
// 		}
// 	}

// 	// POST
// 	if original.Method == "POST" && original.Body != "" {
// 		for _, inj := range injections {
// 			check("POST", "body", original.Body, original.Body+inj, func(payload string) dto.Request {
// 				modified := original
// 				modified.Body = payload
// 				return modified
// 			})
// 		}
// 	}

// 	// Cookies
// 	for key, val := range original.Cookie {
// 		for _, inj := range injections {
// 			strVal := fmt.Sprint(val)
// 			check("Cookie", key, strVal, strVal+inj, func(payload string) dto.Request {
// 				modified := original
// 				modified.Cookie[key] = payload
// 				return modified
// 			})
// 		}
// 	}

// 	// Headers
// 	for key, val := range original.Headers {
// 		for _, inj := range injections {
// 			check("Header", key, val, val+inj, func(payload string) dto.Request {
// 				log.Println("!",val+inj, "!")
// 				modified := original
// 				modified.Headers[key] = payload
// 				return modified
// 			})
// 		}
// 	}

// 	safety_info := "SQL injection – уязвимых параметров не найдено"
// 	safety_status := true
// 	if len(vulnerableParams) > 0 {
// 		safety_info = "SQL injection – уязвимые параметры: " + strings.Join(vulnerableParams, ", ")
// 		safety_status = false
// 	}

// 	full_data := dto.Scanned{
// 		Info:         pair,
// 		SecurityInfo: safety_info,
// 		Safe:         safety_status,
// 		Injections:   triedInjections,
// 	}

// 	w.Header().Set("Content-Type", "text/html")
// 	if err := scan_tmpl.Execute(w, full_data); err != nil {
// 		http.Error(w, "Template execution error", http.StatusInternalServerError)
// 	}
// }

func getContentLength(resp dto.Response) int {
	if cl, ok := resp.Headers["Content-Length"]; ok {
		if parsed, err := strconv.Atoi(cl); err == nil {
			return parsed
		}
	}
	return len(resp.Body)
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

func get_request_from_DB_by_ID(id string) (dto.RequestAndResponse, error) {
	index := -1
	fmt.Sscanf(id, "%d", &index)
	pair, err := database.GetRequestResponseByID(index)
	return pair, err
}

// // handler для проверки уязвимости запросов на SQL injection – во всех GET/POST/Сookie/HTTP заголовках
// func ScanByID(w http.ResponseWriter, r *http.Request) {
// 	log.Println("scanByID started")
// 	vars := mux.Vars(r)
// 	id := vars["id"]

// 	pair, err := get_request_from_DB_by_ID(id)
// 	if err != nil {
// 		http.Error(w, "Invalid ID", http.StatusNotFound)
// 	}
// 	safety_info := "SQL injection – во все GET/POST/Сookie/HTTP заголовки невозможна"
// 	safety_status := true
// 	full_data := dto.Scanned{Info: pair, SecurityInfo: safety_info, Safe: safety_status}
// 	w.Header().Set("Content-Type", "text/html")
// 	if err := scan_tmpl.Execute(w, full_data); err != nil {
// 		http.Error(w, "Template execution error", http.StatusInternalServerError)
// 	}
// }

// func HTTPrerequest(pair dto.RequestAndResponse) (dto.Response, error) {
// 	log.Println("HTTPrerequest started")
// 	original := pair.Request
// 	targetURL := "http://" + original.Headers["Host"] + original.Path

// 	req, err := http.NewRequest(original.Method, targetURL, strings.NewReader(original.Body))
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to build request: %v", err)
// 	}

// 	// Установка заголовков (кроме Host)
// 	for k, v := range original.Headers {
// 		if k != "Host" {
// 			req.Header.Set(k, v)
// 		}
// 	}
// 	req.Host = original.Headers["Host"]
// 	fmt.Println("!!!!!!!!!!!!!!", req.Host)
// 	fmt.Println("@", original)

// 	for name, val := range original.Cookie {
// 		req.AddCookie(&http.Cookie{Name: name, Value: fmt.Sprint(val)})
// 	}

// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to repeat request: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	bodyBytes, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to read response body: %v", err)
// 	}

// 	repeatResp := dto.Response{
// 		Code:    resp.StatusCode,
// 		Message: resp.Status,
// 		Headers: func() map[string]string {
// 			m := make(map[string]string)
// 			for k, v := range resp.Header {
// 				m[k] = strings.Join(v, ", ")
// 			}
// 			return m
// 		}(),
// 		Body: string(bodyBytes),
// 	}
// 	return repeatResp, nil
// }
// работает но не для авторизации
// func HTTPSrerequest(pair dto.RequestAndResponse) (dto.Response, error) {
// 	log.Println("HTTPSrerequest started")
// 	saved := pair.Request
// 	log.Println("saved", saved)
// 	urlStr := "https://" + saved.Path
// 	parsedURL, err := url.Parse(urlStr)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("invalid URL: %v", err)
// 	}

// 	bodyReader := io.NopCloser(strings.NewReader(saved.Body))

// 	req := &http.Request{
// 		Method: saved.Method,
// 		URL:    parsedURL,
// 		Host:   saved.Path,
// 		Header: make(http.Header),
// 		Body:   bodyReader,
// 	}

// 	for k, v := range saved.Headers {
// 		req.Header.Set(k, v)
// 	}

// 	if len(saved.Cookie) > 0 {
// 		var cookies []string
// 		for k, v := range saved.Cookie {
// 			cookies = append(cookies, fmt.Sprintf("%s=%v", k, v))
// 		}
// 		req.Header.Set("Cookie", strings.Join(cookies, "; "))
// 	}
// 	log.Println("rerequested parsedURL.Host", req.Host)
// 	log.Println("resending request - ", req)
// 	conn, err := tls.Dial("tcp", req.Host, &tls.Config{InsecureSkipVerify: true})
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("tls dial error: %v", err)
// 	}
// 	defer conn.Close()

// 	if err := req.Write(conn); err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to write request: %v", err)
// 	}

// 	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to read response: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	respBody, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return dto.Response{}, fmt.Errorf("failed to read response body: %v", err)
// 	}

// 	responseStruct := dto.Response{
// 		Code:    resp.StatusCode,
// 		Message: resp.Status,
// 		Headers: func() map[string]string {
// 			m := make(map[string]string)
// 			for k, v := range resp.Header {
// 				m[k] = strings.Join(v, ", ")
// 			}
// 			return m
// 		}(),
// 		Body: string(respBody),
// 	}

// 	return responseStruct, nil
// }

// func RepeatByID(w http.ResponseWriter, r *http.Request) {
// 	log.Println("repeatByID started")

// 	vars := mux.Vars(r)
// 	id := vars["id"]

// 	pair, err := get_request_from_DB_by_ID(id)
// 	if err != nil {
// 		http.Error(w, "Invalid ID", http.StatusNotFound)
// 		return
// 	}
// 	original := pair.Request
// 	targetURL := "http://" + original.Headers["Host"] + original.Path

// 	req, err := http.NewRequest(original.Method, targetURL, strings.NewReader(original.Body))
// 	if err != nil {
// 		http.Error(w, "failed to build request", http.StatusInternalServerError)
// 		return
// 	}

// 	// Установка заголовков (кроме Host)
// 	for k, v := range original.Headers {
// 		if k != "Host" {
// 			req.Header.Set(k, v)
// 		}
// 	}
// 	req.Host = original.Headers["Host"]
// 	fmt.Println("!!!!!!!!!!!!!!", req.Host)
// 	fmt.Println("@", original)

// 	for name, val := range original.Cookie {
// 		req.AddCookie(&http.Cookie{Name: name, Value: fmt.Sprint(val)})
// 	}

// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		http.Error(w, "failed to repeat request", http.StatusInternalServerError)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	bodyBytes, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		http.Error(w, "failed to read response body", http.StatusInternalServerError)
// 		return
// 	}

// 	repeatResp := dto.Response{
// 		Code:    resp.StatusCode,
// 		Message: resp.Status,
// 		Headers: func() map[string]string {
// 			m := make(map[string]string)
// 			for k, v := range resp.Header {
// 				m[k] = strings.Join(v, ", ")
// 			}
// 			return m
// 		}(),
// 		Body: string(bodyBytes),
// 	}
// 	newData := dto.Rerequested{Old: pair, NewResponse: repeatResp}
// 	w.Header().Set("Content-Type", "text/html")
// 	if err := repeat_request_tmpl.Execute(w, newData); err != nil {
// 		http.Error(w, "Template execution error", http.StatusInternalServerError)
// 	}
// }
