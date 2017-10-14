package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"golang.org/x/net/websocket"
)

const (
	queryLatest = iota
	queryAll
	responseBlockchain
)

var genesisBlock = &Block{
	Index:        0,
	PreviousHash: "0",
	Timestamp:    time.Now().Unix(),
	Data:         "Type:ShareBlock;Useridp:0;Userids:0;Artistid:0;Song:_null_",
	Hash:         "61edfa2d7415d5fac0b29870d0e1ce8fbea237f5a134b0255c41da7a5f4804b8",
}

var (
	sockets      []*websocket.Conn
	blockchain   = []*Block{genesisBlock}
	httpAddr     = flag.String("api", ":4001", "api server address.")
	p2pAddr      = flag.String("p2p", ":7001", "p2p server address.")
	initialPeers = flag.String("peers", "ws://localhost:7001", "initial peers")
)

type Block struct {
	Index        int64  `json:"index"`
	PreviousHash string `json:"previousHash"`
	Timestamp    int64  `json:"timestamp"`
	Data         string `json:"data"`
	Hash         string `json:"hash"`
}

func (b *Block) String() string {
	return fmt.Sprintf("index:%d,previousHash:%s,timestamp:%d,data:%s,hash:%s\n", b.Index, b.PreviousHash, b.Timestamp, b.Data, b.Hash)
}

type ByIndex []*Block

func (b ByIndex) Len() int           { return len(b) }
func (b ByIndex) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByIndex) Less(i, j int) bool { return b[i].Index < b[j].Index }

type ResponseBlockchain struct {
	Type int    `json:"type"`
	Data string `json:"data"`
}

func errFatal(msg string, err error) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}

func connectToPeers(peersAddr []string) {
	for _, peer := range peersAddr {
		if peer == "" {
			continue
		}
		ws, err := websocket.Dial(peer, "", peer)
		if err != nil {
			log.Println("dial to peer", err)
			continue
		}
		initConnection(ws)
	}
}

func initConnection(ws *websocket.Conn) {
	go wsHandleP2P(ws)

	log.Println("query latest block.")
	ws.Write(queryLatestMsg())
}

func handleBlocks(w http.ResponseWriter, r *http.Request) {
	bs, _ := json.Marshal(blockchain)
	w.Write(bs)
}

func queryReturn(id string, seer *session.Session) (result *dynamodb.GetItemOutput) {

	svc := dynamodb.New(seer)

	input := &dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"userid": {
				S: aws.String(id),
			},
		},
		TableName: aws.String("user"),
	}

	result, err := svc.GetItem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				fmt.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				fmt.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				fmt.Println(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return
	}

	return result
}

func parseData(Data string) (Userid string, Amount string, Artistid string, Song string) {

	parseOne := strings.Split(Data, ";")
	parseUID := strings.Split(parseOne[1], ":")
	parseAmt := strings.Split(parseOne[2], ":")
	parseAid := strings.Split(parseOne[3], ":")
	parseS := strings.Split(parseOne[4], ":")

	return parseUID[1], parseAmt[1], parseAid[1], parseS[1]

}

func ExtractArgsWithinBrackets(str string) (res []string) {

	brackets := &unicode.RangeTable{
		R16: []unicode.Range16{
			{0x0028, 0x0029, 1}, // ( )
			{0x005b, 0x005d, 1}, // [ ]
			{0x007b, 0x007d, 1}, // { }
		},
	}

	isBracket := func(r rune) bool {
		if unicode.In(r, brackets) {
			return true
		}
		return false
	}

	args := strings.FieldsFunc(str, isBracket)
	for _, a := range args {
		a = strings.TrimSpace(a)
		a = strings.TrimPrefix(a, ",")
		a = strings.TrimSuffix(a, ",")
		if a != "" {
			res = append(res, "("+a+")")
		}
	}

	return
}

func checkOriginalPurchase(data string) (result int) {

	/*
		Useridp, Userids, Artistid, Song := parseData(data)

		seer, _ := session.NewSession(&aws.Config{
			Region: aws.String("ap-southeast-1"),
		})

		response := queryReturn(Userid, seer)

		res := ExtractArgsWithinBrackets(fmt.Sprintf("%v", response))

		var pos int
		var rgx = regexp.MustCompile(`\"(.*?)\"`)

		for _, r := range res {
			pos = pos + 1
			if r == "(userid:)" {
				break
			} else if r == "(\n    userid:)" {
				break
			}
		}

		rsa := rgx.FindStringSubmatch(res[pos])

		inCredit, err := strconv.Atoi(Amount)
		if err != nil {
			// handle error
			fmt.Println(err)
			os.Exit(2)
		}

		payment, err := strconv.Atoi(rsa[1])
		if err != nil {
			// handle error
			fmt.Println(err)
			os.Exit(2)
		}

		if payment >= inCredit {
			fmt.Println("Enough Money")
			return 0
		}

		fmt.Println("No Money")
		return 1
	*/

	return 0

}

func handleMineBlock(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Data string `json:"data"`
	}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(&v)

	if err != nil {
		w.WriteHeader(http.StatusGone)
		log.Println("[API] invalid block data : ", err.Error())
		w.Write([]byte("invalid block data. " + err.Error() + "\n"))
		return
	}

	if checkOriginalPurchase(v.Data) == 0 {
		block := generateNextBlock(v.Data)
		addBlock(block)
		broadcast(responseLatestMsg())
	} else {
		w.WriteHeader(http.StatusGone)
		log.Println("[API] invalid original transaction: ", err.Error())
		w.Write([]byte("invalid original transaction. " + err.Error() + "\n"))
		return
	}
}

func handlePeers(w http.ResponseWriter, r *http.Request) {
	var slice []string
	for _, socket := range sockets {
		if socket.IsClientConn() {
			slice = append(slice, strings.Replace(socket.LocalAddr().String(), "ws://", "", 1))
		} else {
			slice = append(slice, socket.Request().RemoteAddr)
		}
	}
	bs, _ := json.Marshal(slice)
	w.Write(bs)
}
func handleAddPeer(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Peer string `json:"peer"`
	}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(&v)
	if err != nil {
		w.WriteHeader(http.StatusGone)
		log.Println("[API] invalid peer data : ", err.Error())
		w.Write([]byte("invalid peer data. " + err.Error()))
		return
	}
	connectToPeers([]string{v.Peer})
}

func wsHandleP2P(ws *websocket.Conn) {
	var (
		v    = &ResponseBlockchain{}
		peer = ws.LocalAddr().String()
	)
	sockets = append(sockets, ws)

	for {
		var msg []byte
		err := websocket.Message.Receive(ws, &msg)
		if err == io.EOF {
			log.Printf("p2p Peer[%s] shutdown, remove it form peers pool.\n", peer)
			break
		}
		if err != nil {
			log.Println("Can't receive p2p msg from ", peer, err.Error())
			break
		}

		log.Printf("Received[from %s]: %s.\n", peer, msg)
		err = json.Unmarshal(msg, v)
		errFatal("invalid p2p msg", err)

		switch v.Type {
		case queryLatest:
			v.Type = responseBlockchain

			bs := responseLatestMsg()
			log.Printf("responseLatestMsg: %s\n", bs)
			ws.Write(bs)

		case queryAll:
			d, _ := json.Marshal(blockchain)
			v.Type = responseBlockchain
			v.Data = string(d)
			bs, _ := json.Marshal(v)
			log.Printf("responseChainMsg: %s\n", bs)
			ws.Write(bs)

		case responseBlockchain:
			handleBlockchainResponse([]byte(v.Data))
		}

	}
}

func getLatestBlock() (block *Block) { return blockchain[len(blockchain)-1] }

func responseLatestMsg() (bs []byte) {
	var v = &ResponseBlockchain{Type: responseBlockchain}
	d, _ := json.Marshal(blockchain[len(blockchain)-1:])
	v.Data = string(d)
	bs, _ = json.Marshal(v)
	return
}

func queryLatestMsg() []byte { return []byte(fmt.Sprintf("{\"type\": %d}", queryLatest)) }

func queryAllMsg() []byte { return []byte(fmt.Sprintf("{\"type\": %d}", queryAll)) }

func calculateHashForBlock(b *Block) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d%s%d%s", b.Index, b.PreviousHash, b.Timestamp, b.Data))))
}

func generateNextBlock(data string) (nb *Block) {
	var previousBlock = getLatestBlock()
	nb = &Block{
		Data:         data,
		PreviousHash: previousBlock.Hash,
		Index:        previousBlock.Index + 1,
		Timestamp:    time.Now().Unix(),
	}
	nb.Hash = calculateHashForBlock(nb)
	return
}

func addBlock(b *Block) {
	if isValidNewBlock(b, getLatestBlock()) {
		blockchain = append(blockchain, b)
	}
}

func isValidNewBlock(nb, pb *Block) (ok bool) {
	if nb.Hash == calculateHashForBlock(nb) &&
		pb.Index+1 == nb.Index &&
		pb.Hash == nb.PreviousHash {
		ok = true
	}
	return
}

func isValidChain(bc []*Block) bool {
	if bc[0].String() != genesisBlock.String() {
		log.Println("No same GenesisBlock.", bc[0].String())
		return false
	}
	var temp = []*Block{bc[0]}
	for i := 1; i < len(bc); i++ {
		if isValidNewBlock(bc[i], temp[i-1]) {
			temp = append(temp, bc[i])
		} else {
			return false
		}
	}
	return true
}

func replaceChain(bc []*Block) {
	if isValidChain(bc) && len(bc) > len(blockchain) {
		log.Println("Received blockchain is valid. Replacing current blockchain with received blockchain.")
		blockchain = bc
		broadcast(responseLatestMsg())
	} else {
		log.Println("Received blockchain invalid.")
	}
}

func broadcast(msg []byte) {
	for n, socket := range sockets {
		_, err := socket.Write(msg)
		if err != nil {
			log.Printf("peer [%s] disconnected.", socket.RemoteAddr().String())
			sockets = append(sockets[0:n], sockets[n+1:]...)
		}
	}
}

func handleBlockchainResponse(msg []byte) {
	var receivedBlocks = []*Block{}

	err := json.Unmarshal(msg, &receivedBlocks)
	errFatal("invalid blockchain", err)

	sort.Sort(ByIndex(receivedBlocks))

	latestBlockReceived := receivedBlocks[len(receivedBlocks)-1]
	latestBlockHeld := getLatestBlock()
	if latestBlockReceived.Index > latestBlockHeld.Index {
		log.Printf("blockchain possibly behind. We got: %d Peer got: %d", latestBlockHeld.Index, latestBlockReceived.Index)
		if latestBlockHeld.Hash == latestBlockReceived.PreviousHash {
			log.Println("We can append the received block to our chain.")
			blockchain = append(blockchain, latestBlockReceived)
		} else if len(receivedBlocks) == 1 {
			log.Println("We have to query the chain from our peer.")
			broadcast(queryAllMsg())
		} else {
			log.Println("Received blockchain is longer than current blockchain.")
			replaceChain(receivedBlocks)
		}
	} else {
		log.Println("received blockchain is not longer than current blockchain. Do nothing.")
	}
}

func main() {
	flag.Parse()
	connectToPeers(strings.Split(*initialPeers, ","))

	http.HandleFunc("/blocks", handleBlocks)
	http.HandleFunc("/mine_block", handleMineBlock)
	http.HandleFunc("/peers", handlePeers)
	http.HandleFunc("/add_peer", handleAddPeer)
	go func() {
		log.Println("Listen HTTP on", *httpAddr)
		errFatal("start api server", http.ListenAndServe(*httpAddr, nil))
	}()

	http.Handle("/", websocket.Handler(wsHandleP2P))
	log.Println("Listen P2P on ", *p2pAddr)
	errFatal("start p2p server", http.ListenAndServe(*p2pAddr, nil))
}
