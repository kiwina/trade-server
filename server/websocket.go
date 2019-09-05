package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"

	"github.com/coinexchain/trade-server/core"
)

const (
	Subscribe   = "subscribe"
	Unsubscribe = "unsubscribe"
	Ping        = "ping"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type OpCommand struct {
	Op    string   `json:"op"`
	Args  []string `json:"args"`
	Depth int      `json:"depth"`
}

func ServeWsHandleFn(wsManager *core.WebsocketManager, hub *core.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error(err)
			return
		}
		wsConn := core.NewConn(c)
		wsManager.AddConn(wsConn)

		go func() {
			for {
				_, message, err := wsConn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.WithError(err).Error("unexpected close error")
					}
					err = wsManager.CloseConn(wsConn)
					if err != nil {
						log.WithError(err).Error("close websocket failed")
					}
					break
				}

				var command OpCommand
				if err := json.Unmarshal(message, &command); err != nil {
					log.WithError(err).Error("unmarshal message failed")
					continue
				}

				switch command.Op {
				case Subscribe:
					for _, subTopic := range command.Args {
						err = wsManager.AddSubscribeConn(subTopic, command.Depth, wsConn, hub)
						if err != nil {
							log.WithError(err).Error(fmt.Sprintf("Subscribe topic (%s) failed ", subTopic))
						}
					}
				case Unsubscribe:
					for _, subTopic := range command.Args {
						err = wsManager.RemoveSubscribeConn(subTopic, wsConn)
						if err != nil {
							log.WithError(err).Error(fmt.Sprintf("Unsubscribe topic (%s) failed ", subTopic))
						}
					}
				case Ping:
					if err = wsConn.PongHandler()(`{"type": "pong"}`); err != nil {
						log.WithError(err).Error(fmt.Sprintf("pong message failed"))
					}
				}

			}
		}()
	}
}
