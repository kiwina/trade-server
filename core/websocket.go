package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	SeparateArgu = ":"
	MinArguNum   = 0
	MaxArguNum   = 2
)

type ImplSubscriber struct {
	*Conn
	value interface{}
}

func (i ImplSubscriber) Detail() interface{} {
	return i.value
}

func (i ImplSubscriber) WriteMsg(v []byte) error {
	return i.WriteMessage(websocket.TextMessage, v)
}

type Conn struct {
	*websocket.Conn
	topicWithParams map[string]map[string]struct{} // topic --> params
}

func NewConn(c *websocket.Conn) *Conn {
	c.SetPingHandler(func(appData string) error {
		return c.WriteMessage(websocket.TextMessage, []byte(appData))
	})
	return &Conn{
		Conn:            c,
		topicWithParams: make(map[string]map[string]struct{}),
	}
}

type WebsocketManager struct {
	sync.RWMutex

	SkipPushed     bool
	connWithTopics map[*Conn]map[string]struct{} // conn --> topics
	topicAndConns  map[string]map[*Conn]struct{}
}

func NewWebSocketManager() *WebsocketManager {
	return &WebsocketManager{
		topicAndConns:  make(map[string]map[*Conn]struct{}),
		connWithTopics: make(map[*Conn]map[string]struct{}),
	}
}

func (w *WebsocketManager) SetSkipOption(isSkip bool) {
	w.SkipPushed = isSkip
}

func (w *WebsocketManager) AddConn(c *Conn) {
	w.Lock()
	defer w.Unlock()
	w.connWithTopics[c] = make(map[string]struct{})
}

func (w *WebsocketManager) CloseConn(c *Conn) error {
	w.Lock()
	defer w.Unlock()
	topics, ok := w.connWithTopics[c]
	if !ok {
		panic("the remove conn not cache in websocketManager ")
	}

	for topic := range topics {
		conns, ok := w.topicAndConns[topic]
		if ok {
			delete(conns, c)
		}
	}
	delete(w.connWithTopics, c)
	return c.Close()
}

func (w *WebsocketManager) AddSubscribeConn(subscriptionTopic string, depth int, c *Conn, hub *Hub) error {
	w.Lock()
	defer w.Unlock()
	values := strings.Split(subscriptionTopic, SeparateArgu)
	if len(values) < 1 || len(values) > MaxArguNum+1 {
		return fmt.Errorf("Expect range of parameters [%d, %d], actual : %d ", MinArguNum, MaxArguNum, len(values)-1)
	}

	topic := values[0]
	params := values[1:]
	if !checkTopicValid(topic, params) {
		log.Errorf("The subscribed topic [%s] is illegal ", topic)
		return fmt.Errorf("The subscribed topic [%s] is illegal ", topic)
	}

	if err := PushFullInformation(subscriptionTopic, depth, c, hub); err != nil {
		return err
	}

	if len(params) != 0 {
		if len(c.topicWithParams[topic]) == 0 {
			c.topicWithParams[topic] = make(map[string]struct{})
		}
		if len(params) == 1 {
			c.topicWithParams[topic][params[0]] = struct{}{}
		} else {
			c.topicWithParams[topic][strings.Join(params, SeparateArgu)] = struct{}{}
		}
	}
	if len(w.topicAndConns[topic]) == 0 {
		w.topicAndConns[topic] = make(map[*Conn]struct{})
	}
	w.topicAndConns[topic][c] = struct{}{}
	w.connWithTopics[c][topic] = struct{}{}

	return nil
}

func (w *WebsocketManager) RemoveSubscribeConn(subscriptionTopic string, c *Conn) error {
	w.Lock()
	defer w.Unlock()
	values := strings.Split(subscriptionTopic, SeparateArgu)
	if len(values) < 1 || len(values) > MaxArguNum+1 {
		return fmt.Errorf("Expect range of parameters [%d, %d], actual : %d ", MinArguNum, MaxArguNum, len(values)-1)
	}
	topic := values[0]
	params := values[1:]
	if !checkTopicValid(topic, params) {
		log.Errorf("The subscribed topic [%s] is illegal ", topic)
		return fmt.Errorf("The subscribed topic [%s] is illegal ", topic)
	}

	if conns, ok := w.topicAndConns[topic]; ok {
		if _, ok := conns[c]; ok {
			if len(params) != 0 {
				if len(params) == 1 {
					delete(c.topicWithParams[topic], params[0])
				} else {
					tmpVal := strings.Join(params, SeparateArgu)
					delete(c.topicWithParams[topic], tmpVal)
				}
				if len(c.topicWithParams[topic]) == 0 {
					delete(conns, c)
				}
			} else {
				delete(conns, c)
			}
		}
	}
	if topics, ok := w.connWithTopics[c]; ok {
		if _, ok := topics[topic]; ok {
			if conns, ok := w.topicAndConns[topic]; ok {
				if _, ok := conns[c]; !ok {
					delete(topics, topic)
				}
			}
		}
	}
	return nil
}

func checkTopicValid(topic string, params []string) bool {
	switch topic {
	case BlockInfoKey, SlashKey:
		if len(params) != 0 {
			return false
		}
		return true
	case TickerKey, UnbondingKey, RedelegationKey, LockedKey,
		UnlockKey, TxKey, IncomeKey, OrderKey, CommentKey,
		BancorTradeKey, BancorKey, DealKey:
		if len(params) != 1 {
			return false
		}
		return true
	case KlineKey:
		if len(params) != 2 {
			return false
		}
		switch params[1] {
		case MinuteStr, HourStr, DayStr:
			return true
		default:
			return false
		}
	case DepthKey:
		if len(params) == 1 {
			return true
		} else if len(params) == 2 {
			switch params[1] {
			case "all", "0.00000001", "0.0000001", "0.000001", "0.00001",
				"0.0001", "0.001", "0.01", "0.1", "1", "10", "100":
				return true
			default:
				return false
			}
		}
		return false
	default:
		return false
	}
}

func getCount(depth int) int {
	depth = limitCount(depth)
	if depth == 0 {
		depth = 10
	}
	return depth
}

func PushFullInformation(subscriptionTopic string, depth int, c *Conn, hub *Hub) error {
	values := strings.Split(subscriptionTopic, SeparateArgu)
	topic, params := values[0], values[1:]
	depth = getCount(depth)

	var err error
	type queryFunc func(string, int64, int64, int) ([]json.RawMessage, []int64)
	queryAndPushFunc := func(typeKey string, param string, qf queryFunc) error {
		data, _ := qf(param, hub.currBlockTime.Unix(), hub.sid, depth)
		for _, v := range data {
			msg := []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", typeKey, string(v)))
			err = c.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				return err
			}
		}
		return nil
	}
	depthLevel := "all"
	if len(params) == 2 && topic == DepthKey {
		depthLevel = params[1]
	}

	switch topic {
	case SlashKey:
		err = querySlashAndPush(hub, c, depth)
	case KlineKey:
		err = queryKlineAndpush(hub, c, params, depth)
	case DepthKey:
		err = queryDepthAndPush(hub, c, params[0], depthLevel, depth)
	case OrderKey:
		err = queryOrderAndPush(hub, c, params[0], depth)
	case TickerKey:
		err = queryTickerAndPush(hub, c, params[0])
	case TxKey:
		err = queryAndPushFunc(TxKey, params[0], hub.QueryTx)
	case LockedKey:
		err = queryAndPushFunc(LockedKey, params[0], hub.QueryLocked)
	case UnlockKey:
		err = queryAndPushFunc(UnlockKey, params[0], hub.QueryUnlock)
	case IncomeKey:
		err = queryAndPushFunc(IncomeKey, params[0], hub.QueryIncome)
	case DealKey:
		err = queryAndPushFunc(DealKey, params[0], hub.QueryDeal)
	case BancorKey:
		err = queryAndPushFunc(BancorKey, params[0], hub.QueryBancorInfo)
	case BancorTradeKey:
		err = queryAndPushFunc(BancorTradeKey, params[0], hub.QueryBancorTrade)
	case RedelegationKey:
		err = queryAndPushFunc(RedelegationKey, params[0], hub.QueryRedelegation)
	case UnbondingKey:
		err = queryAndPushFunc(UnbondingKey, params[0], hub.QueryUnbonding)
	case CommentKey:
		err = queryAndPushFunc(CommentKey, params[0], hub.QueryComment)
	}
	return err
}

func queryTickerAndPush(hub *Hub, c *Conn, market string) error {
	tickers := hub.QueryTickers([]string{market})
	baseData, err := json.Marshal(tickers)
	if err != nil {
		return err
	}
	err = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("{\"type\":\"%s\","+
		" \"payload\":%s}", OrderKey, string(baseData))))
	return err
}

func queryOrderAndPush(hub *Hub, c *Conn, account string, depth int) error {
	data, tags, _ := hub.QueryOrder(account, hub.currBlockTime.Unix(), hub.sid, depth)
	if len(data) != len(tags) {
		return errors.Errorf("The number of orders and tags is not equal")
	}
	for i := len(data) - 1; i >= 0; i-- {
		var msg []byte
		if tags[i] == CreateOrderEndByte {
			msg = []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", CreateOrderKey, string(data[i])))
		} else if tags[i] == FillOrderEndByte {
			msg = []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", FillOrderKey, string(data[i])))
		} else if tags[i] == CancelOrderEndByte {
			msg = []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", CancelOrderKey, string(data[i])))
		}
		err := c.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func queryDepthAndPush(hub *Hub, c *Conn, market string, level string, depth int) error {
	var msg []byte
	sell, buy := hub.QueryDepth(market, depth)
	if level == "all" {
		depRes := DepthDetails{
			TradingPair: market,
			Bids:        buy,
			Asks:        sell,
		}
		bz, err := json.Marshal(depRes)
		if err != nil {
			return err
		}
		msg = []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", DepthKey, string(bz)))
		return c.WriteMessage(websocket.TextMessage, msg)
	}

	sellLevels := mergePrice(sell)
	buyLevels := mergePrice(buy)
	levelBuys := encodeDepthLevels(market, buyLevels, true)
	levelSells := encodeDepthLevels(market, sellLevels, false)
	if v, ok := levelSells[level]; ok && len(v) != 0 {
		msg = []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", DepthKey, string(v)))
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			return err
		}
	}
	if v, ok := levelBuys[level]; ok && len(v) != 0 {
		msg = []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", DepthKey, string(v)))
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			return err
		}
	}

	return nil
}

func queryKlineAndpush(hub *Hub, c *Conn, params []string, depth int) error {
	candleBz := hub.QueryCandleStick(params[0], GetSpanFromSpanStr(params[1]), hub.currBlockTime.Unix(), hub.sid, depth)
	for _, v := range candleBz {
		msg := []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", KlineKey, string(v)))
		err := c.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func querySlashAndPush(hub *Hub, c *Conn, depth int) error {
	data, _ := hub.QuerySlash(hub.currBlockTime.Unix(), hub.sid, depth)
	for _, v := range data {
		msg := []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", SlashKey, string(v)))
		err := c.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *WebsocketManager) GetSlashSubscribeInfo() []Subscriber {
	w.RLock()
	defer w.RUnlock()
	conns := w.topicAndConns[SlashKey]
	res := make([]Subscriber, 0, len(conns))
	for conn := range conns {
		res = append(res, ImplSubscriber{Conn: conn})
	}
	return res
}

func (w *WebsocketManager) GetHeightSubscribeInfo() []Subscriber {
	w.RLock()
	defer w.RUnlock()
	conns := w.topicAndConns[BlockInfoKey]
	res := make([]Subscriber, 0, len(conns))
	for conn := range conns {
		res = append(res, ImplSubscriber{Conn: conn})
	}
	return res
}

func (w *WebsocketManager) GetTickerSubscribeInfo() []Subscriber {
	w.RLock()
	defer w.RUnlock()
	conns := w.topicAndConns[TickerKey]
	res := make([]Subscriber, 0, len(conns))
	for conn := range conns {
		res = append(res, ImplSubscriber{
			Conn:  conn,
			value: conn.topicWithParams[TickerKey],
		})
	}

	return res
}

func (w *WebsocketManager) GetCandleStickSubscribeInfo() map[string][]Subscriber {
	w.RLock()
	defer w.RUnlock()
	conns := w.topicAndConns[KlineKey]
	res := make(map[string][]Subscriber)
	for conn := range conns {
		params := conn.topicWithParams[KlineKey]
		for p := range params {
			vals := strings.Split(p, SeparateArgu)
			res[vals[0]] = append(res[vals[0]], ImplSubscriber{
				Conn:  conn,
				value: vals[1],
			})
		}
	}

	return res
}

func (w *WebsocketManager) getNoDetailSubscribe(topic string) map[string][]Subscriber {
	w.RLock()
	defer w.RUnlock()
	conns := w.topicAndConns[topic]
	res := make(map[string][]Subscriber)
	for conn := range conns {
		for param := range conn.topicWithParams[topic] {
			res[param] = append(res[param], ImplSubscriber{
				Conn: conn,
			})
		}
	}

	return res
}

func (w *WebsocketManager) GetDepthSubscribeInfo() map[string][]Subscriber {
	w.RLock()
	defer w.RUnlock()
	conns := w.topicAndConns[DepthKey]
	res := make(map[string][]Subscriber)
	for conn := range conns {
		params := conn.topicWithParams[DepthKey]
		for p := range params {
			level := "all"
			vals := strings.Split(p, SeparateArgu)
			if len(vals) == 2 {
				level = vals[1]
			}
			res[vals[0]] = append(res[vals[0]], ImplSubscriber{
				Conn:  conn,
				value: level,
			})
		}
	}
	return res
}

func (w *WebsocketManager) GetDealSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(DealKey)
}
func (w *WebsocketManager) GetBancorInfoSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(BancorKey)
}
func (w *WebsocketManager) GetCommentSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(CommentKey)
}
func (w *WebsocketManager) GetOrderSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(OrderKey)
}
func (w *WebsocketManager) GetBancorTradeSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(BancorTradeKey)
}
func (w *WebsocketManager) GetIncomeSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(IncomeKey)
}
func (w *WebsocketManager) GetUnbondingSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(UnbondingKey)
}
func (w *WebsocketManager) GetRedelegationSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(RedelegationKey)
}
func (w *WebsocketManager) GetUnlockSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(UnlockKey)
}
func (w *WebsocketManager) GetTxSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(TxKey)
}

func (w *WebsocketManager) GetLockedSubscribeInfo() map[string][]Subscriber {
	return w.getNoDetailSubscribe(LockedKey)
}

// Push msgs----------------------------
func (w *WebsocketManager) sendEncodeMsg(subscriber Subscriber, typeKey string, info []byte) {
	if !w.SkipPushed {
		msg := []byte(fmt.Sprintf("{\"type\":\"%s\", \"payload\":%s}", typeKey, string(info)))
		if err := subscriber.WriteMsg(msg); err != nil {
			log.Errorf(err.Error())
			s := subscriber.(ImplSubscriber)
			if err = w.CloseConn(s.Conn); err != nil {
				log.Error(err)
			}
		}
	}
}
func (w *WebsocketManager) PushLockedSendMsg(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, LockedKey, info)
}
func (w *WebsocketManager) PushSlash(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, SlashKey, info)
}
func (w *WebsocketManager) PushHeight(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, BlockInfoKey, info)
}
func (w *WebsocketManager) PushTicker(subscriber Subscriber, t []*Ticker) {
	payload, err := json.Marshal(t)
	if err != nil {
		log.Error(err)
		return
	}
	w.sendEncodeMsg(subscriber, TickerKey, payload)
}
func (w *WebsocketManager) PushDepthSell(subscriber Subscriber, delta []byte) {
	w.sendEncodeMsg(subscriber, DepthKey, delta)
}
func (w *WebsocketManager) PushDepthBuy(subscriber Subscriber, delta []byte) {
	w.sendEncodeMsg(subscriber, DepthKey, delta)
}
func (w *WebsocketManager) PushCandleStick(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, KlineKey, info)
}
func (w *WebsocketManager) PushDeal(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, DealKey, info)
}
func (w *WebsocketManager) PushCreateOrder(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, CreateOrderKey, info)
}
func (w *WebsocketManager) PushFillOrder(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, FillOrderKey, info)
}
func (w *WebsocketManager) PushCancelOrder(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, CancelOrderKey, info)
}
func (w *WebsocketManager) PushBancorInfo(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, BancorKey, info)
}
func (w *WebsocketManager) PushBancorTrade(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, BancorTradeKey, info)
}
func (w *WebsocketManager) PushIncome(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, IncomeKey, info)
}
func (w *WebsocketManager) PushUnbonding(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, UnbondingKey, info)
}
func (w *WebsocketManager) PushRedelegation(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, RedelegationKey, info)
}
func (w *WebsocketManager) PushUnlock(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, UnlockKey, info)
}
func (w *WebsocketManager) PushTx(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, TxKey, info)
}
func (w *WebsocketManager) PushComment(subscriber Subscriber, info []byte) {
	w.sendEncodeMsg(subscriber, CommentKey, info)
}
