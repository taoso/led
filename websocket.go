package led

import (
	"log"
	"net"

	"github.com/gorilla/websocket"
)

func (p *Proxy) ProxyWS(ws *websocket.Conn) {
	c, err := net.Dial("tcp", "127.0.0.1:8000")
	if err != nil {
		log.Println("websocket upstream dial err:", err)
		return
	}
	defer c.Close()

	go func() {
		var buf [1024]byte
		for {
			n, err := c.Read(buf[:])
			if err != nil {
				log.Println("websocket upstream read err:", err)
				return
			}
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				log.Println("websocket downstream write err:", err)
				return
			}
		}
	}()

	for {
		_, m, err := ws.ReadMessage()
		if err != nil {
			log.Println("websocket downstream read err:", err)
			return
		}
		_, err = c.Write(m)
		if err != nil {
			log.Println("websocket upstream write err:", err)
			return
		}
	}
}
