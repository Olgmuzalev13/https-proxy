package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

type InMemory struct {
	Request  *http.Request
	Response *http.Response
}

func handleHTTP(w http.ResponseWriter, r *http.Request, db []InMemory) {
	fmt.Println("all", r)
	fmt.Println("URL", r.URL)
	fmt.Println("Proto", r.Proto)
	fmt.Println("Host", r.Host)
	fmt.Println("Header", r.Header)
	fmt.Println("Method", r.Method)
	fmt.Println("Body", r.Body)

	var user_request *http.Request
	user_request = r
	user_request.URL.Host = user_request.Host
	user_request.URL.Scheme = "http"
	delete(user_request.Header, "Proxy-Connection")
	fmt.Println("ready", user_request)
	//client := http.Client{}
	//r1, _ := http.NewRequest(r.Method,  "http://"+r.Host+"/", r.Body)
	//resp, err := client.Do(r1)
	resp, err := http.DefaultTransport.RoundTrip(user_request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	var text []byte
	a, err := resp.Body.Read(text)
	fmt.Println("http ok", text, a, resp.StatusCode)
	db = append(db, InMemory{
		Request:  user_request,
		Response: resp,
	})
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func handleHTTPS(w http.ResponseWriter, r *http.Request, db []InMemory) {

}

func main() {
	db := []InMemory{}
	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Println("Method", r.Method)
			if r.Method == http.MethodConnect {
				handleHTTPS(w, r, db)
			} else {
				handleHTTP(w, r, db)
			}
		}),
	}
	log.Println("Started at http://127.0.0.1:8080")
	if err := server.ListenAndServe(); err != nil {
		log.Println("Server failed:", err)
	}
}
