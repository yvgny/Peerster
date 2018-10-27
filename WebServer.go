package main

import (
	"encoding/json"
	"fmt"
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

type ServerInfo struct {
	Id      string
	Address string
}
type Peers struct {
	Peers []string
}
type Contacts struct {
	Contacts []string
}

func NewWebServer(g *Gossiper) *WebServer {

	r := mux.NewRouter()
	r.HandleFunc("/message", g.getMessagesHandler).Methods("GET")
	r.HandleFunc("/message", g.postMessageHandler).Methods("POST")
	r.HandleFunc("/private-message", g.postPrivateMessageHandler).Methods("POST")
	r.HandleFunc("/node", g.getNodesHandler).Methods("GET")
	r.HandleFunc("/node", g.addNodeHandler).Methods("POST")
	r.HandleFunc("/contacts", g.getContactsHandler).Methods("GET")
	r.HandleFunc("/id", g.idHandler).Methods("GET")
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
		Id:      g.name,
		Address: g.gossipAddress.String(),
	}
	marshal, err := json.Marshal(msg)
	if err == nil {
		_, err = writer.Write(marshal)
		if err != nil {
			writeErrorToHTTP(writer, err)
			fmt.Println(err.Error())
		}
	} else {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) getNodesHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	msg := Peers{
		Peers: g.peers.Elements(),
	}

	bytes, err := json.Marshal(msg)
	if err == nil {
		_, err = writer.Write(bytes)
		if err != nil {
			writeErrorToHTTP(writer, err)
			fmt.Println(err.Error())
		}
	} else {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) getContactsHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	msg := Contacts{
		Contacts: g.routingTable.Elements(),
	}

	bytes, err := json.Marshal(msg)
	if err == nil {
		_, err = writer.Write(bytes)
		if err != nil {
			writeErrorToHTTP(writer, err)
			fmt.Println(err.Error())
		}
	} else {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) addNodeHandler(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		http.Error(writer, err.Error(), 500)
	}
	nodeIP := request.Form.Get("IP")
	port := request.Form.Get("Port")

	err = g.AddPeer(nodeIP + ":" + port)
	if err != nil {
		fmt.Println(err.Error())
		http.Error(writer, err.Error(), 500)
	}
}

func (g *Gossiper) postPrivateMessageHandler(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		http.Error(writer, err.Error(), 500)
	}
	messsage := request.Form.Get("Text")
	dest := request.Form.Get("Destination")

	err = g.sendPrivateMessage(dest, messsage)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) postMessageHandler(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		http.Error(writer, err.Error(), 500)
	}
	messsage := request.Form.Get("Message")
	rumor := common.RumorMessage{
		Text: messsage,
	}
	gossip := common.GossipPacket{
		Rumor: &rumor,
	}
	err = g.HandleClientMessage(&gossip)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) getMessagesHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	array := make([]common.RumorMessage, 0)
	g.messages.Range(func(key, value interface{}) bool {
		msg := value.(common.RumorMessage)

		// Remove route rumor messages
		if msg.Text != "" {
			array = append(array, msg)
		}
		return true
	})

	bytes, err := json.Marshal(array)
	if err == nil {
		_, err = writer.Write(bytes)
		if err != nil {
			writeErrorToHTTP(writer, err)
		}
	} else {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
}

func writeErrorToHTTP(writer http.ResponseWriter, err error) {
	http.Error(writer, err.Error(), 500)
}
