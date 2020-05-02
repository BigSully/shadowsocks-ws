package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/BigSully/shadowsocks-ws/ws"
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

	u, err := url.Parse(strings.Trim(server, "'"))
	if err != nil {
		panic(err)
	}

	urlStr := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Path)
	key := u.User.Username()
	//urlStr="ws://localhost:3000/"
	fmt.Println(urlStr)
	fmt.Println(key)

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

			conn, err := ws.Dial(urlStr, ws.Auth(key, ""))

			if err != nil {
				logf("failed to connect to server ", err)
				return
			}
			defer conn.Close()
			go conn.Ping()

			if _, err = conn.WriteAddress(tgt); err != nil {
				return
			}

			logf("proxy %s <-> %s <-> %s", c.RemoteAddr(), server, tgt)

			relayws(*conn, c)
		}()
	}
}

// Listen on addr for incoming connections.
func tcpRemote(addr string, shadow func(net.Conn) net.Conn) {
	u, err := url.Parse(strings.Trim(addr, "'"))
	if err != nil {
		panic(err)
	}

	//key := u.User.Username()
	host := u.Host

	ws.Listen(host, func(c *ws.Conn, remoteAddr string) {
		go func() {
			defer c.Close()

			r, err := c.ReadAddress()
			if err != nil {
				logf("error to read target address: %v", err)
				return
			}

			tgt, err := socks.ReadAddr(r)
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

			logf("proxy %s <-> %s", remoteAddr, tgt)
			relayws(*c, rc)
		}()
	})
}

func relayws(left ws.Conn, right net.Conn) {
	go func() {
		left.ReadFrom(right)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	}()
	left.WriteTo(right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
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
