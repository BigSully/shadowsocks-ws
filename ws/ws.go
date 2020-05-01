package ws

import (
	"encoding/base64"
	"github.com/gorilla/websocket"
	"net"
	"net/http"

	"log"
	"time"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 30 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

func Auth(username string, password string) (h http.Header) {
	h = http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))}}
	return
}

type Conn struct {
	conn *websocket.Conn
}

// Dial: addr should be in the form of host:port
func Dial(urlStr string, h http.Header) (conn *Conn, err error) {
	c, _, err := websocket.DefaultDialer.Dial(urlStr, h)
	if err != nil {
		return
	}

	conn = &Conn{conn: c}

	return
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Ping() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait)); err != nil {
				log.Println("ping:", err)
			}
		}
	}
}

func (c *Conn) WriteAddress(p []byte) (n int, err error) {
	if err := c.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		log.Println("error write address:", err)
	}

	return
}

func (c *Conn) ReadFrom(src net.Conn) {
	buf := leakyBuf.Get()
	defer leakyBuf.Put(buf)
	for {
		if n, err := src.Read(buf); err != nil {
			log.Println("error read net: ", err)
			break
		} else {
			if err := c.conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				log.Println("error write ws:", err)
				break
			}
		}

	}
	return
}

func (c *Conn) WriteTo(dst net.Conn) {
	for {
		mt, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("ws read:", err)

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error read from ws: %v", err)
			}
			break
		}
		if mt != websocket.BinaryMessage {
			log.Printf("error need to deal with message type in error: %v", err)
		}

		if _, err := dst.Write(message); err != nil {
			log.Println("error write to net:", err)
			break
		}
	}
	return
}
