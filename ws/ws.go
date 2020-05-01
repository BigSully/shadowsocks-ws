package ws

import (
	"encoding/base64"
	"github.com/gorilla/websocket"
	"io"
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

func (c Conn) Write(p []byte) (n int, err error) {
	w, err := c.conn.NextWriter(websocket.BinaryMessage) // get a writer everytime there is a copy and close it when exit
	if err != nil {
		return
	}
	defer w.Close()
	return w.Write(p)
}

func (c Conn) Read(p []byte) (n int, err error) {
	_, r, err := c.conn.NextReader()
	if err != nil {
		return
	}
	return r.Read(p)
}

func (c *Conn) ReadFrom(src net.Conn) {
	if n, err := io.Copy(c, src); err != nil { // implicit loop in copy
		log.Println("error copy net to ws:", n, err)
		return
	}
	log.Println("out ------ ")
}

func (c *Conn) WriteTo(dst net.Conn) {
	for {
		_, r, err := c.conn.NextReader()
		if err != nil {
			break
		}

		if n, err := io.Copy(dst, r); err != nil { // implicit loop in copy
			log.Println("error write to net:", n, err)
			break
		}
	}
}
