package main

import (
	"github.com/gorilla/mux"
	"net/http"
	"time"
)

const LocalAddress = "127.0.0.1"

type WebServer struct {
	router   *mux.Router
	srv      *http.Server
}

func NewWebServer() *WebServer {

	r := mux.NewRouter()
	// r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("html/static/"))))
	r.HandleFunc("/message", messageHandler)
	r.HandleFunc("/node", nodeHandler)
	r.HandleFunc("/id", idHandler)
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("html/"))))

	server := &http.Server{
		Addr:         LocalAddress + ":8080",
		Handler:      r,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	ws := &WebServer{
		router:   r,
		srv:      server,
	}

	return ws
}

func (ws *WebServer) StartWebServer() {
	go ws.srv.ListenAndServe()
}

func idHandler(writer http.ResponseWriter, request *http.Request) {

}

func nodeHandler(writer http.ResponseWriter, request *http.Request) {

}

func messageHandler(writer http.ResponseWriter, request *http.Request) {

}
