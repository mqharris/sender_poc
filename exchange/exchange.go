package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
)

type Event struct {
	Price  float32 `json:"price,string"`
	Amount float32 `json:"amount,string"`
	Symbol string  `json:"symbol"`
}

type Message struct {
	Timestamp int     `json:"timestamp"`
	Events    []Event `json:"events"`
}

var done chan interface{}
var interrupt chan os.Signal

func receiveHandler(connection *websocket.Conn, kafkaConnection *kafka.Conn) {
	defer close(done)
	for {
		_, msg, err := connection.ReadMessage()
		if err != nil {
			log.Println("Error in receive:", err)
			return
		}

		var message Message
		json.Unmarshal(msg, &message)
		messageJSON, _ := json.Marshal(message) // ol' in-out out-in
		kafkaConnection.SetWriteDeadline(time.Now().Add(1 * time.Second))
		kafkaConnection.WriteMessages(kafka.Message{Value: []byte(messageJSON)})
		fmt.Println("sent to kafka", message.Events)
	}
}

func processMessage(mt int, p []byte) {
	// log.Printf("Received: %s\n", p)
	var message Message
	json.Unmarshal(p, &message)
	// fmt.Printf("%s\n", p)
	// fmt.Printf("ts = %v\n", message.timestamp)
	fmt.Println(message.Events)
}

func main() {

	topic := "coins"
	partition := 0

	kafkaConn, err := kafka.DialLeader(context.Background(), "tcp", "localhost:9092", topic, partition)
	if err != nil {
		log.Fatal("failed to dial leader:", err)
	}

	done = make(chan interface{})    // Channel to indicate that the receiverHandler is done
	interrupt = make(chan os.Signal) // Channel to listen for interrupt signal to terminate gracefully

	signal.Notify(interrupt, os.Interrupt) // Notify the interrupt channel for SIGINT

	socketUrl := "wss://api.gemini.com/v1/multimarketdata"

	ws, err := url.Parse(socketUrl)
	if err != nil {
		log.Fatal((err))
	}
	values := ws.Query()

	values.Add("symbols", "BTCUSD,ETHUSD")
	values.Add("bids", "false")
	values.Add("offers", "false")
	values.Add("auctions", "false")
	values.Add("trades", "true")

	ws.RawQuery = values.Encode()

	fmt.Println(ws.String())

	conn, _, err := websocket.DefaultDialer.Dial(ws.String(), nil)
	if err != nil {
		log.Fatal("Error connecting to Websocket Server:", err)
	}
	defer conn.Close()
	defer kafkaConn.Close()
	go receiveHandler(conn, kafkaConn)

	// Our main loop for the client
	// We send our relevant packets here
	for {
		select {
		case <-time.After(time.Duration(1) * time.Millisecond * 1000):
			// // Send an echo packet every second
			// err := conn.WriteMessage(websocket.TextMessage, []byte("Hello from GolangDocs!"))
			// if err != nil {
			// 	log.Println("Error during writing to websocket:", err)
			// 	return
			// }

		case <-interrupt:
			// We received a SIGINT (Ctrl + C). Terminate gracefully...
			log.Println("Received SIGINT interrupt signal. Closing all pending connections")

			// Close our websocket connection
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Error during closing websocket:", err)
				return
			}

			select {
			case <-done:
				log.Println("Receiver Channel Closed! Exiting....")
			case <-time.After(time.Duration(1) * time.Second):
				log.Println("Timeout in closing receiving channel. Exiting....")
			}
			return
		}
	}
}
