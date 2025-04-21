package handlers

import (
	"fmt"
	"html/template"
	"httpproxy/database"
	"httpproxy/dto"
	"io"
	"log"
	"net/http"
	"path/filepath"
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
	original := pair.Request
	targetURL := "http://" + original.Headers["Host"] + original.Path

	req, err := http.NewRequest(original.Method, targetURL, strings.NewReader(original.Body))
	if err != nil {
		http.Error(w, "failed to build request", http.StatusInternalServerError)
		return
	}

	// Установка заголовков (кроме Host)
	for k, v := range original.Headers {
		if k != "Host" {
			req.Header.Set(k, v)
		}
	}
	req.Host = original.Headers["Host"]
	fmt.Println("!!!!!!!!!!!!!!", req.Host)
	fmt.Println("@", original)

	for name, val := range original.Cookie {
		req.AddCookie(&http.Cookie{Name: name, Value: fmt.Sprint(val)})
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "failed to repeat request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read response body", http.StatusInternalServerError)
		return
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
	newData := dto.Rerequested{Old: pair, NewResponse: repeatResp}
	w.Header().Set("Content-Type", "text/html")
	if err := repeat_request_tmpl.Execute(w, newData); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// handler для проверки уязвимости запросов на SQL injection – во всех GET/POST/Сookie/HTTP заголовках
func ScanByID(w http.ResponseWriter, r *http.Request) {
	log.Println("scanByID started")
	vars := mux.Vars(r)
	id := vars["id"]

	pair, err := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
	}
	safety_info := "SQL injection – во все GET/POST/Сookie/HTTP заголовки невозможна"
	safety_status := true
	full_data := dto.Scanned{Info: pair, SecurityInfo: safety_info, Safe: safety_status}
	w.Header().Set("Content-Type", "text/html")
	if err := scan_tmpl.Execute(w, full_data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
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

