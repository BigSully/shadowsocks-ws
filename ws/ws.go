package ws

import (
	"bytes"
	"encoding/base64"
	"errors"
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
				return
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

//// p might not able to hold all data from r
//func (c Conn) Read(p []byte) (n int, err error) {
//	_, r, err := c.conn.NextReader()
//	if err != nil {
//		return
//	}
//	return r.Read(p)
//}

func (c Conn) ReadAddress() (r io.Reader, err error) {
	_, p, err := c.conn.ReadMessage()
	if err != nil {
		return
	}
	r = bytes.NewReader(p)
	return
}

func (c *Conn) ReadFrom(src net.Conn) {
	if n, err := io.Copy(c, src); err != nil { // implicit loop in copy
		log.Println("error copy net to ws:", n, err)
		return
	}
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

func ReadUserIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}
func Listen(addr string, handleConnection func(conn *Conn, remoteAddr string)) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var upgrader = websocket.Upgrader{} // use default options
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		remoteAddr := ReadUserIP(r)
		handleConnection(&Conn{conn: c}, remoteAddr)
	})
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
