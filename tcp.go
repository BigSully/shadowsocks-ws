package main

import (
	"io"
	"net"
	"time"

	"github.com/BigSully/shadowsocks-ws/ws"
	"github.com/gorilla/websocket"
	"github.com/shadowsocks/go-shadowsocks2/socks"
)

// Create a SOCKS server listening on addr and proxy to server.
func socksLocal(addr, server string, shadow func(net.Conn) net.Conn) {
	logf("SOCKS proxy %s <-> %s", addr, server)
	tcpLocal(addr, server, shadow, func(c net.Conn) (socks.Addr, error) { return socks.Handshake(c) })
}

// Create a TCP tunnel from addr to target via server.
func tcpTun(addr, server, target string, shadow func(net.Conn) net.Conn) {
	tgt := socks.ParseAddr(target)
	if tgt == nil {
		logf("invalid target address %q", target)
		return
	}
	logf("TCP tunnel %s <-> %s <-> %s", addr, server, target)
	tcpLocal(addr, server, shadow, func(net.Conn) (socks.Addr, error) { return tgt, nil })
}

// Listen on addr and proxy to server to reach target from getAddr.
func tcpLocal(addr, server string, shadow func(net.Conn) net.Conn, getAddr func(net.Conn) (socks.Addr, error)) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %s", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			tgt, err := getAddr(c)
			if err != nil {

				// UDP: keep the connection until disconnect then free the UDP socket
				if err == socks.InfoUDPAssociate {
					buf := make([]byte, 1)
					// block here
					for {
						_, err := c.Read(buf)
						if err, ok := err.(net.Error); ok && err.Timeout() {
							continue
						}
						logf("UDP Associate End.")
						return
					}
				}

				logf("failed to get target address: %v", err)
				return
			}

			//wsAddr := fmt.Sprintf("ws://%v:%v/", config["server"], config["server_port"])
			wsAddr := "ws://localhost:3000/"
			method := "aes-256-cfb"
			password := "123456"
			cipher, err := ws.NewCipher(method, password)

			//rc, err := net.Dial("tcp", server)
			wsConn, err := ws.Dial(wsAddr)
			if err != nil {
				logf("failed to connect to server2 %v: %v", wsAddr, err)
				return
			}
			defer wsConn.Close()

			newConn := ws.NewConn(wsConn, cipher)
			if err != nil {
				logger.Println("error connecting to shadowsocks server:", err)
				return
			}

			//rc.(*net.TCPConn).SetKeepAlive(true)  // TODO: pinging
			//rc = shadow(rc)   // TODO: encryption  can be skipped for the moment

			// tell the distant server the real server we want to access and help the distant server initialize a new connection
			if _, err = newConn.Write(tgt); err != nil {
				wsConn.Close()
				return
			}

			logf("proxy %s <-> %s <-> %s", c.RemoteAddr(), server, tgt)
			relayws(*newConn, c)
			//relay3(*raw, c)

			//if err != nil {
			//	logf("failed to connect to server %v: %v", server, err)
			//	return
			//}
			//defer rc.Close()
			//rc.(*net.TCPConn).SetKeepAlive(true)
			//rc = shadow(rc)
			//
			//if _, err = rc.Write(tgt); err != nil {
			//	logf("failed to send target address: %v", err)
			//	return
			//}
			//
			//logf("proxy %s <-> %s <-> %s", c.RemoteAddr(), server, tgt)
			//_, _, err = relay(rc, c)
			//if err != nil {
			//	if err, ok := err.(net.Error); ok && err.Timeout() {
			//		return // ignore i/o timeout
			//	}
			//	logf("relay error: %v", err)
			//}
		}()
	}
}

// Listen on addr for incoming connections.
func tcpRemote(addr string, shadow func(net.Conn) net.Conn) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}

	logf("listening TCP on %s", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			c = shadow(c)

			tgt, err := socks.ReadAddr(c)
			if err != nil {
				logf("failed to get target address: %v", err)
				return
			}

			rc, err := net.Dial("tcp", tgt.String())
			if err != nil {
				logf("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)

			logf("proxy %s <-> %s", c.RemoteAddr(), tgt)
			_, _, err = relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logf("relay error: %v", err)
			}
		}()
	}
}

func relay3(left websocket.Conn, right net.Conn) {
	go ws.RelayNet2Ws(right, left)
	ws.RelayWs2Net(left, right)
}

// relay copies between left and right bidirectionally. Returns number of
//// bytes copied from right to left, from left to right, and any error occurred.
//func relay2(left ws.Conn, right net.Conn) (int64, int64, error) {
//	type res struct {
//		N   int64
//		Err error
//	}
//	ch := make(chan res)
//
//	go func() {
//		n, err := io.Copy(right, left)
//		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
//		//left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
//		ch <- res{n, err}
//	}()
//
//	n, err := io.Copy(left, right)
//	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
//	//left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
//	rs := <-ch
//
//	if err == nil {
//		err = rs.Err
//	}
//	return n, rs.N, err
//}

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relayws(left ws.Conn, right net.Conn) {
	type res struct {
		N   int64
		Err error
	}
	//ch := make(chan res)

	go ws.PipeNet2WS(right, left)
	ws.PipeWS2Net(left, right)
	//logger.Println("closed connection to", hostPort)

	//
	//go func() {
	//	n, err := io.Copy(right, left)
	//	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	//	//left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	//	ch <- res{n, err}
	//}()
	//
	//n, err := io.Copy(left, right)
	//right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	////left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	//rs := <-ch
	//
	//if err == nil {
	//	err = rs.Err
	//}
	//return n, rs.N, err
}

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relay(left, right net.Conn) (int64, int64, error) {
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		n, err := io.Copy(right, left)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()

	n, err := io.Copy(left, right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch

	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}
