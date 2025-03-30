package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("all", r)
	fmt.Println("URL", r.URL)
	fmt.Println("Proto", r.Proto)
	fmt.Println("Host", r.Host)
	fmt.Println("Header", r.Header)
	fmt.Println("Method", r.Method)
	var user_request *http.Request
	user_request = r
	var cut_url *url.URL
	cut_url = r.URL
	fmt.Println("Scheme", r.URL.Scheme, r.URL.Opaque, r.URL.User, r.URL.Host, "Path", r.URL.Path, r.URL.RawPath, r.URL.OmitHost, r.URL.ForceQuery, r.URL.RawQuery, r.URL.Fragment, r.URL.RawFragment)
	cut_url.Scheme = ""
	cut_url.Path = ""
	user_request.URL = cut_url
	delete(user_request.Header, "Proxy-Connection")
	fmt.Println("ready", user_request)
	client := http.Client{}
	r1, _ := http.NewRequest(r.Method,  "http://"+r.Host+"/", r.Body)
	resp, err := client.Do(r1)
	//resp, err := http.DefaultTransport.RoundTrip(user_request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	var text []byte
	a, err := resp.Body.Read(text)
	fmt.Println("http ok", text, a, resp.StatusCode)
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

func main() {
	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleHTTP(w, r)
		}),
	}
	log.Fatal(server.ListenAndServe())
}
