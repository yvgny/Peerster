package gossiper

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/yvgny/Peerster/common"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
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
	r.HandleFunc("/private-message", g.getPrivateMessagesHandler).Methods("GET")
	r.HandleFunc("/index-file", g.postFileToIndexHandler).Methods("POST")
	r.HandleFunc("/download-file", g.postFileToDownload).Methods("POST")
	r.HandleFunc("/cloud-file", g.postCloudFile).Methods("POST")
	r.HandleFunc("/cloud-file", g.getCloudFilesHandler).Methods("GET")
	r.HandleFunc("/search-file", g.postSearchForFile).Methods("POST")
	r.HandleFunc("/matched-files", g.getMatchedFiles).Methods("GET")
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

func (g *Gossiper) getCloudFilesHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	bytes, err := json.Marshal(g.cloudStorage.GetAllMappings())
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

func (g *Gossiper) getMatchedFiles(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	bytes, err := json.Marshal(g.data.getAllRemoteMatches())
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
func (g *Gossiper) getPrivateMessagesHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	msg := g.privateMessages.Elements()

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
		writeErrorToHTTP(writer, err)
	}
	nodeIP := request.Form.Get("IP")
	port := request.Form.Get("Port")

	err = g.AddPeer(nodeIP + ":" + port)
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
}

func (g *Gossiper) postSearchForFile(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
	keywordsStr := request.Form.Get("Keywords")
	budgetStr := request.Form.Get("Budget")
	keywords := strings.Split(keywordsStr, ",")
	budget, err := strconv.ParseUint(budgetStr, 10, 64)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	} else {
		sr := common.SearchRequest{
			Budget:   budget,
			Origin:   g.name,
			Keywords: keywords,
		}
		go g.searchRemoteFile(&sr)
	}
}

func (g *Gossiper) postPrivateMessageHandler(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
	messsage := request.Form.Get("Text")
	dest := request.Form.Get("Destination")

	err = g.sendPrivateMessage(dest, messsage)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) postCloudFile(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
	filename := request.Form.Get("Filename")
	err = g.HandleClientCloudRequest(filename, g.blockchain)
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
}

func (g *Gossiper) postFileToIndexHandler(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
	filename := request.Form.Get("Filename")

	file, err := g.data.addLocalFile(filepath.Join(common.SharedFilesFolder, filename), nil)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
	hexHash, _ := json.Marshal(file.MetaHash)
	_, err = writer.Write(hexHash)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) postFileToDownload(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
	filename := request.Form.Get("Filename")
	user := request.Form.Get("User")
	metafileHash := request.Form.Get("HashValue")

	hash, err := hex.DecodeString(metafileHash)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}

	var hashArray [32]byte
	copy(hashArray[:], hash)
	err = g.downloadFile(user, hashArray, filename, nil)
	if err != nil {
		writeErrorToHTTP(writer, err)
		fmt.Println(err.Error())
	}
	hexHash, _ := json.Marshal(hex.EncodeToString(hash))
	_, err = writer.Write(hexHash)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func (g *Gossiper) postMessageHandler(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		writeErrorToHTTP(writer, err)
	}
	messsage := request.Form.Get("Message")
	rumor := common.RumorMessage{
		Text: messsage,
	}
	clientPacket := common.ClientPacket{}
	clientPacket.Rumor = &rumor
	err = g.HandleClientRumorMessage(&clientPacket)
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
