package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/yvgny/Peerster/common"
	"net/http"
	"time"
)

type WebServer struct {
	router   *mux.Router
	gossiper *Gossiper
	srv      *http.Server
}

type Message struct {
	message string
}

type ServerInfo struct {
	id      string
	address string
}

func NewWebServer(g *Gossiper) *WebServer {

	r := mux.NewRouter()
	// r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("html/static/"))))
	r.HandleFunc("/message", g.getMessagesHandler).Methods("GET")
	r.HandleFunc("/message", g.postMessageHandler).Methods("POST")
	r.HandleFunc("/node", g.nodeHandler)
	r.HandleFunc("/id", g.idHandler)
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("html/"))))

	server := &http.Server{
		Addr:         common.LocalAddress + ":8080",
		Handler:      r,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	ws := &WebServer{
		router:   r,
		gossiper: g,
		srv:      server,
	}

	return ws
}

func (ws *WebServer) StartWebServer() {
	go ws.srv.ListenAndServe()
}

func (g *Gossiper) idHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	msg := ServerInfo{
		id:      g.name,
		address: g.gossipAddress.String(),
	}
	marshal, err := json.Marshal(msg)
	if err == nil {
		writer.Write(marshal)
	}
}

func (g *Gossiper) nodeHandler(writer http.ResponseWriter, request *http.Request) {

}

func (g *Gossiper) postMessageHandler(writer http.ResponseWriter, request *http.Request) {
	request.ParseForm()
	messsage := request.Form.Get("message")
	rumor := common.RumorMessage{
		Text: messsage,
	}
	gossip := common.GossipPacket{
		Rumor: &rumor,
	}
	g.HandleClientMessage(&gossip)
}

func (g *Gossiper) getMessagesHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	array := make([]common.RumorMessage, 0)
	g.messages.Range(func(key, value interface{}) bool {
		msg := value.(common.RumorMessage)
		array = append(array, msg)
		return true
	})

	bytes, err := json.Marshal(array)
	if err == nil {
		writer.Write(bytes)
	}
}
