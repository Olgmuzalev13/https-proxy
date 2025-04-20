package handlers

import (
	"fmt"
	"html/template"
	"httpproxy/database"
	"httpproxy/dto"
	"log"
	"net/http"
	"path/filepath"

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

// handler для повторной отправки запроса
func RepeatByID(w http.ResponseWriter, r *http.Request) {
	log.Println("repeatByID started")
	vars := mux.Vars(r)
	id := vars["id"]
	pair, err := get_request_from_DB_by_ID(id)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusNotFound)
	}

	w.Header().Set("Content-Type", "text/html")
	if err := repeat_request_tmpl.Execute(w, pair); err != nil {
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

	w.Header().Set("Content-Type", "text/html")
	if err := scan_tmpl.Execute(w, pair); err != nil {
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

// var db = dto.InMemoryDB{
// 	[]dto.RequestAndResponse{{
// 		Request: dto.Request{
// 			Method: "POST",
// 			Path:   "/path1/path2",
// 			GetParams: map[string]any{
// 				"x": 123,
// 				"y": "qwe",
// 			},
// 			Headers: map[string]string{
// 				"Host":   "example.org",
// 				"Header": "value",
// 			},
// 			Cookie: map[string]any{
// 				"cookie1": 1,
// 				"cookie2": "qwe",
// 			},
// 			Body: "<html>...",
// 		},
// 		Response: dto.Response{
// 			Code:    200,
// 			Message: "OK",
// 			Headers: map[string]string{
// 				"Server": "nginx/1.14.1",
// 				"Header": "value",
// 			},
// 			Body: "<html>...",
// 		},
// 	}},
// }
