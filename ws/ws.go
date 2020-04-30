package ws

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net"

	//"io"
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

type Conn struct {
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	Send chan []byte

	// Buffered channel of inbound messages.
	Recv chan []byte

	*Cipher
}

// Dial: addr should be in the form of host:port
func Dial(addr string, cipher *Cipher) (conn *Conn, err error) {
	c, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return
	}

	conn = &Conn{
		conn:   c,
		Send:   make(chan []byte, 256),
		Recv:   make(chan []byte, 256),
		Cipher: cipher}

	go conn.writePump()
	go conn.readPump()

	//go conn.Ping()

	return
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Ping() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Conn) readPump() {
	defer func() {
		c.conn.Close()
	}()
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		c.Recv <- message
	}
}

// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Conn) ReadAll() (b []byte, n int, err error) {
	b = <-c.Recv // block if no data is available
	n = len(b)

	return
}

func (c *Conn) Write(p []byte) (n int, err error) {
	c.Send <- p
	n = len(p)

	return
}

func (c *Conn) RelayFrom(src net.Conn) {
	buf := leakyBuf.Get()
	defer leakyBuf.Put(buf)
	for {
		if n, err := src.Read(buf); err != nil {
			//log.Println("Net -> Ws Read: ", n, err)
			fmt.Sprintf("net Read, len: %d, err: %s", n, err)
			break
		} else {
			if err := c.conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				//log.Println("Net -> Ws write:", err)
				fmt.Sprintf("ws Write, len: %d, err: %s", n, err)
				break
			}
		}

	}
	return
}

func (c *Conn) RelayTo(dst net.Conn) {
	for {
		mt, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("ws read:", err)

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error when reading: %v", err)
			}
			break
		}
		if mt != websocket.BinaryMessage {
			log.Printf("WS -> Net need to deal with message type in error: %v", err)
		}

		if _, err := dst.Write(message); err != nil {
			log.Println("net write:", err)
			break
		}
	}
	return
}
