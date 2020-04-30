package ws

import (
	"github.com/gorilla/websocket"
	"log"
	"net"
)

func RelayNet2Ws(src net.Conn, dst websocket.Conn) {
	buf := leakyBuf.Get()
	defer leakyBuf.Put(buf)
	for {
		if _, err := src.Read(buf); err != nil {
			log.Println("Net -> Ws Read:", err)
			break
		}
		if err := dst.WriteMessage(websocket.BinaryMessage, buf); err != nil {
			log.Println("Net -> Ws write:", err)
			break
		}

	}
	return
}

func RelayWs2Net(src websocket.Conn, dst net.Conn) {
	for {
		mt, message, err := src.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error when reading: %v", err)
			}
			break
		}
		if mt != websocket.BinaryMessage {
			log.Printf("WS -> Net need to deal with message type in error: %v", err)
		}

		if _, err := dst.Write(message); err != nil {
			log.Println("WS -> Net write:", err)
			break
		}
	}
	return
}
