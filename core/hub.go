package core

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coinexchain/dex/app"
	"github.com/coinexchain/dex/modules/authx"
	"github.com/coinexchain/dex/modules/bancorlite"
	"github.com/coinexchain/dex/modules/comment"
	"github.com/coinexchain/dex/modules/market"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dbm "github.com/tendermint/tm-db"
)

const (
	MaxCount = 1024
	//These bytes are used as the first byte in key
	CandleStickByte  = byte(0x10)
	DealByte         = byte(0x12)
	OrderByte        = byte(0x14)
	BancorInfoByte   = byte(0x16)
	BancorTradeByte  = byte(0x18)
	IncomeByte       = byte(0x1A)
	TxByte           = byte(0x1C)
	CommentByte      = byte(0x1D)
	BlockHeightByte  = byte(0x20)
	DetailByte       = byte(0x22)
	RedelegationByte = byte(0x30)
	UnbondingByte    = byte(0x32)
	UnlockByte       = byte(0x34)
)

func limitCount(count int) int {
	if count > MaxCount {
		return MaxCount
	}
	return count
}

func int64ToBigEndianBytes(n int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n))
	return b[:]
}

// Following are some functions to generate keys to access the KVStore
func (hub *Hub) getKeyFromBytesAndTime(firstByte byte, bz []byte, lastByte byte, unixTime int64) []byte {
	res := make([]byte, 0, 1+1+len(bz)+1+16+1)
	res = append(res, firstByte)
	res = append(res, byte(len(bz)))
	res = append(res, bz...)
	res = append(res, byte(0))
	res = append(res, int64ToBigEndianBytes(unixTime)...) //the block's time at which the KV pair is generated
	res = append(res, int64ToBigEndianBytes(hub.sid)...)  // the serial ID for a KV pair
	res = append(res, lastByte)
	return res
}

func (hub *Hub) getKeyFromBytes(firstByte byte, bz []byte, lastByte byte) []byte {
	return hub.getKeyFromBytesAndTime(firstByte, bz, lastByte, hub.currBlockTime.Unix())
}

func getStartKeyFromBytes(firstByte byte, bz []byte) []byte {
	res := make([]byte, 0, 1+1+len(bz)+1)
	res = append(res, firstByte)
	res = append(res, byte(len(bz)))
	res = append(res, bz...)
	res = append(res, byte(0))
	return res
}

func getEndKeyFromBytes(firstByte byte, bz []byte, time int64, sid int64) []byte {
	res := make([]byte, 0, 1+1+len(bz)+1+8)
	res = append(res, firstByte)
	res = append(res, byte(len(bz)))
	res = append(res, bz...)
	res = append(res, byte(0))
	res = append(res, int64ToBigEndianBytes(time)...)
	res = append(res, int64ToBigEndianBytes(sid)...)
	return res
}

//==========

func (hub *Hub) getCandleStickKey(market string, timespan byte) []byte {
	bz := append([]byte(market), []byte{0, timespan}...)
	return hub.getKeyFromBytes(CandleStickByte, bz, 0)
}

func getCandleStickEndKey(market string, timespan byte, endTime int64, sid int64) []byte {
	bz := append([]byte(market), []byte{0, timespan}...)
	return getEndKeyFromBytes(CandleStickByte, bz, endTime, sid)
}

func getCandleStickStartKey(market string, timespan byte) []byte {
	bz := append([]byte(market), []byte{0, timespan}...)
	return getStartKeyFromBytes(CandleStickByte, bz)
}

func (hub *Hub) getDealKey(market string) []byte {
	return hub.getKeyFromBytes(DealByte, []byte(market), 0)
}
func (hub *Hub) getBancorInfoKey(market string) []byte {
	return hub.getKeyFromBytes(BancorInfoByte, []byte(market), 0)
}
func (hub *Hub) getCommentKey(token string) []byte {
	return hub.getKeyFromBytes(CommentByte, []byte(token), 0)
}

func (hub *Hub) getCreateOrderKey(addr string) []byte {
	return hub.getKeyFromBytes(OrderByte, []byte(addr), CreateOrderEndByte)
}
func (hub *Hub) getFillOrderKey(addr string) []byte {
	return hub.getKeyFromBytes(OrderByte, []byte(addr), FillOrderEndByte)
}
func (hub *Hub) getCancelOrderKey(addr string) []byte {
	return hub.getKeyFromBytes(OrderByte, []byte(addr), CancelOrderEndByte)
}
func (hub *Hub) getBancorTradeKey(addr string) []byte {
	return hub.getKeyFromBytes(BancorTradeByte, []byte(addr), byte(0))
}
func (hub *Hub) getIncomeKey(addr string) []byte {
	return hub.getKeyFromBytes(IncomeByte, []byte(addr), byte(0))
}
func (hub *Hub) getTxKey(addr string) []byte {
	return hub.getKeyFromBytes(TxByte, []byte(addr), byte(0))
}
func (hub *Hub) getRedelegationEventKey(addr string, time int64) []byte {
	return hub.getKeyFromBytesAndTime(RedelegationByte, []byte(addr), byte(0), time)
}
func (hub *Hub) getUnbondingEventKey(addr string, time int64) []byte {
	return hub.getKeyFromBytesAndTime(UnbondingByte, []byte(addr), byte(0), time)
}
func (hub *Hub) getUnlockEventKey(addr string) []byte {
	return hub.getKeyFromBytes(UnlockByte, []byte(addr), byte(0))
}

type (
	NewHeightInfo                    = market.NewHeightInfo
	CreateOrderInfo                  = market.CreateOrderInfo
	FillOrderInfo                    = market.FillOrderInfo
	CancelOrderInfo                  = market.CancelOrderInfo
	NotificationSlash                = app.NotificationSlash
	TransferRecord                   = app.TransferRecord
	NotificationTx                   = app.NotificationTx
	NotificationBeginRedelegation    = app.NotificationBeginRedelegation
	NotificationBeginUnbonding       = app.NotificationBeginUnbonding
	NotificationCompleteRedelegation = app.NotificationCompleteRedelegation
	NotificationCompleteUnbonding    = app.NotificationCompleteUnbonding
	NotificationUnlock               = authx.NotificationUnlock
	TokenComment                     = comment.TokenComment
	CommentRef                       = comment.CommentRef
	MsgBancorTradeInfoForKafka       = bancorlite.MsgBancorTradeInfoForKafka
	MsgBancorInfoForKafka            = bancorlite.MsgBancorInfoForKafka
)

type TripleManager struct {
	sell *DepthManager
	buy  *DepthManager
	tman *TickerManager
}

type Hub struct {
	db            dbm.DB
	batch         dbm.Batch
	dbMutex       sync.RWMutex
	tickerMutex   sync.RWMutex
	depthMutex    sync.RWMutex
	subMan        SubscribeManager
	managersMap   map[string]TripleManager
	csMan         CandleStickManager
	currBlockTime time.Time
	lastBlockTime time.Time
	tickerMap     map[string]*Ticker
	sid           int64 // the serial ID for a KV pair
}

var _ Querier = &Hub{}
var _ Consumer = &Hub{}

func NewHub(db dbm.DB, subMan SubscribeManager) Hub {
	return Hub{
		db:            db,
		batch:         db.NewBatch(),
		subMan:        subMan,
		managersMap:   make(map[string]TripleManager),
		csMan:         NewCandleStickManager(nil),
		currBlockTime: time.Unix(0, 0),
		lastBlockTime: time.Unix(0, 0),
		tickerMap:     make(map[string]*Ticker),
	}
}

func (hub *Hub) HasMarket(market string) bool {
	_, ok := hub.managersMap[market]
	return ok
}

func (hub *Hub) AddMarket(market string) {
	hub.managersMap[market] = TripleManager{
		sell: DefaultDepthManager(),
		buy:  DefaultDepthManager(),
		tman: DefaultTickerManager(market),
	}
	hub.csMan.AddMarket(market)
}

func (hub *Hub) ConsumeMessage(msgType string, bz []byte) {
	switch msgType {
	case "height_info":
		hub.handleNewHeightInfo(bz)
	case "notify_slash":
		hub.handleNotificationSlash(bz)
	case "notify_tx":
		hub.handleNotificationTx(bz)
	case "begin_redelegation":
		hub.handleNotificationBeginRedelegation(bz)
	case "begin_unbonding":
		hub.handleNotificationBeginUnbonding(bz)
	case "complete_redelegation":
		hub.handleNotificationCompleteRedelegation(bz)
	case "complete_unbonding":
		hub.handleNotificationCompleteUnbonding(bz)
	case "notify_unlock":
		hub.handleNotificationUnlock(bz)
	case "token_comment":
		hub.handleTokenComment(bz)
	case "create_order_info":
		hub.handleCreateOrderInfo(bz)
	case "fill_order_info":
		hub.handleFillOrderInfo(bz)
	case "del_order_info":
		hub.handleCancelOrderInfo(bz)
	case "bancor_trade":
		hub.handleMsgBancorTradeInfoForKafka(bz)
	case "bancor_info":
		hub.handleMsgBancorInfoForKafka(bz)
	case "commit":
		hub.commit()
	default:
		hub.Log(fmt.Sprintf("Unknown Message Type:%s", msgType))
	}
}

func (hub *Hub) Log(s string) {
	fmt.Print(s)
}

func (hub *Hub) handleNewHeightInfo(bz []byte) {
	var v NewHeightInfo
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NewHeightInfo")
		return
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b[:], uint64(v.TimeStamp.Unix()))
	key := append([]byte{BlockHeightByte}, int64ToBigEndianBytes(v.Height)...)
	hub.batch.Set(key, b)

	for _, ss := range hub.subMan.GetHeightSubscribeInfo() {
		hub.subMan.PushHeight(ss, bz)
	}

	hub.lastBlockTime = hub.currBlockTime
	hub.currBlockTime = v.TimeStamp
	hub.beginForCandleSticks()
}

func (hub *Hub) beginForCandleSticks() {
	candleSticks := hub.csMan.NewBlock(hub.currBlockTime)
	var triman TripleManager
	var targets []Subscriber
	sym := ""
	var ok bool
	currMinute := hub.currBlockTime.Hour() * hub.currBlockTime.Minute()
	for _, cs := range candleSticks {
		if sym != cs.Market {
			triman, ok = hub.managersMap[cs.Market]
			if !ok {
				sym = ""
				continue
			}
			info := hub.subMan.GetCandleStickSubscribeInfo()
			sym = cs.Market
			targets, ok = info[sym]
			if !ok {
				sym = ""
				continue
			}
		}
		if len(sym) == 0 {
			continue
		}
		// Update tickers' prices
		if cs.TimeSpan == Minute {
			triman.tman.UpdateNewestPrice(cs.ClosePrice, currMinute)
		}
		// Push candle sticks to subscribers
		for _, target := range targets {
			timespan, ok := target.Detail().(byte)
			if !ok || timespan != cs.TimeSpan {
				continue
			}
			hub.subMan.PushCandleStick(target, &cs)
		}
		// Save candle sticks to KVStore
		key := hub.getCandleStickKey(cs.Market, cs.TimeSpan)
		bz, err := json.Marshal(cs)
		if err != nil {
			continue
		}
		hub.batch.Set(key, bz)
		hub.sid++
	}
}

func (hub *Hub) handleNotificationSlash(bz []byte) {
	for _, ss := range hub.subMan.GetSlashSubscribeInfo() {
		hub.subMan.PushSlash(ss, bz)
	}
}

func (hub *Hub) handleNotificationTx(bz []byte) {
	var v NotificationTx
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NotificationTx")
		return
	}
	snBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(snBytes[:], uint64(v.SerialNumber))

	key := append([]byte{DetailByte}, snBytes...)
	hub.batch.Set(key, bz)
	hub.sid++

	for _, acc := range v.Signers {
		signer := acc.String()
		k := hub.getTxKey(signer)
		hub.batch.Set(k, snBytes)
		hub.sid++

		info := hub.subMan.GetTxSubscribeInfo()
		targets, ok := info[signer]
		if !ok {
			continue
		}
		for _, target := range targets {
			hub.subMan.PushTx(target, bz)
		}
	}

	for _, transRec := range v.Transfers {
		recipient := transRec.Recipient
		k := hub.getIncomeKey(recipient)
		hub.batch.Set(k, snBytes)
		hub.sid++

		info := hub.subMan.GetIncomeSubscribeInfo()
		targets, ok := info[recipient]
		if !ok {
			continue
		}
		for _, target := range targets {
			hub.subMan.PushIncome(target, bz)
		}
	}
}
func (hub *Hub) handleNotificationBeginRedelegation(bz []byte) {
	var v NotificationBeginRedelegation
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NotificationBeginRedelegation")
		return
	}
	t, err := time.Parse(time.RFC3339, v.CompletionTime)
	if err != nil {
		return
	}
	key := hub.getRedelegationEventKey(v.Delegator, t.Unix())
	hub.batch.Set(key, bz)
	hub.sid++
}
func (hub *Hub) handleNotificationBeginUnbonding(bz []byte) {
	var v NotificationBeginUnbonding
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NotificationBeginUnbonding")
		return
	}
	t, err := time.Parse(time.RFC3339, v.CompletionTime)
	if err != nil {
		return
	}
	key := hub.getUnbondingEventKey(v.Delegator, t.Unix())
	hub.batch.Set(key, bz)
	hub.sid++
}
func (hub *Hub) handleNotificationCompleteRedelegation(bz []byte) {
	var v NotificationCompleteRedelegation
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NotificationCompleteRedelegation")
		return
	}
	info := hub.subMan.GetRedelegationSubscribeInfo()
	targets, ok := info[v.Delegator]
	if !ok {
		return
	}
	end := hub.getRedelegationEventKey(v.Delegator, hub.currBlockTime.Unix())
	start := hub.getRedelegationEventKey(v.Delegator, hub.lastBlockTime.Unix()-1)
	hub.dbMutex.RLock()
	iter := hub.db.ReverseIterator(start, end)
	defer func() {
		iter.Close()
		hub.dbMutex.RUnlock()
	}()
	for ; iter.Valid(); iter.Next() {
		for _, target := range targets {
			hub.subMan.PushRedelegation(target, iter.Value())
		}
	}
}
func (hub *Hub) handleNotificationCompleteUnbonding(bz []byte) {
	var v NotificationCompleteUnbonding
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NotificationCompleteUnbonding")
		return
	}
	info := hub.subMan.GetUnbondingSubscribeInfo()
	targets, ok := info[v.Delegator]
	if !ok {
		return
	}
	end := hub.getUnbondingEventKey(v.Delegator, hub.currBlockTime.Unix())
	start := hub.getUnbondingEventKey(v.Delegator, hub.lastBlockTime.Unix()-1)
	hub.dbMutex.RLock()
	iter := hub.db.ReverseIterator(start, end)
	defer func() {
		iter.Close()
		hub.dbMutex.RUnlock()
	}()
	for ; iter.Valid(); iter.Next() {
		for _, target := range targets {
			hub.subMan.PushUnbonding(target, iter.Value())
		}
	}
}
func (hub *Hub) handleNotificationUnlock(bz []byte) {
	var v NotificationUnlock
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal NotificationUnlock")
		return
	}
	info := hub.subMan.GetUnlockSubscribeInfo()
	targets, ok := info[v.Address.String()]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushUnlock(target, bz)
	}
}
func (hub *Hub) handleTokenComment(bz []byte) {
	var v TokenComment
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal TokenComment")
		return
	}
	key := hub.getCommentKey(v.Token)
	hub.batch.Set(key, bz)
	hub.sid++
	info := hub.subMan.GetCommentSubscribeInfo()
	targets, ok := info[v.Token]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushComment(target, bz)
	}
}
func (hub *Hub) handleCreateOrderInfo(bz []byte) {
	var v CreateOrderInfo
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal CreateOrderInfo")
		return
	}
	if !hub.HasMarket(v.TradingPair) {
		hub.AddMarket(v.TradingPair)
	}
	key := hub.getCreateOrderKey(v.Sender)
	hub.batch.Set(key, bz)
	hub.sid++
	info := hub.subMan.GetOrderSubscribeInfo()
	targets, ok := info[v.Sender]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushCreateOrder(target, bz)
	}

	managers, ok := hub.managersMap[v.TradingPair]
	if !ok {
		return
	}
	amount := sdk.NewInt(v.Quantity)
	hub.depthMutex.Lock()
	defer func() {
		hub.depthMutex.Unlock()
	}()
	if v.Side == market.SELL {
		managers.sell.DeltaChange(v.Price, amount)
	} else {
		managers.buy.DeltaChange(v.Price, amount)
	}
}
func (hub *Hub) handleFillOrderInfo(bz []byte) {
	var v FillOrderInfo
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal FillOrderInfo")
		return
	}
	info := hub.subMan.GetOrderSubscribeInfo()
	accAndSeq := strings.Split(v.OrderID, "-")
	if len(accAndSeq) != 2 {
		return
	}
	key := hub.getFillOrderKey(accAndSeq[0])
	hub.batch.Set(key, bz)
	hub.sid++
	targets, ok := info[accAndSeq[0]]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushFillOrder(target, bz)
	}

	csRec := hub.csMan.GetRecord(v.TradingPair)
	if csRec == nil {
		return
	}
	price := sdk.NewDec(v.DealMoney).QuoInt64(v.DealStock)
	csRec.Update(hub.currBlockTime, price, v.DealStock)

	managers, ok := hub.managersMap[v.TradingPair]
	if !ok {
		return
	}
	negStock := sdk.NewInt(-v.DealStock)
	hub.depthMutex.Lock()
	defer func() {
		hub.depthMutex.Unlock()
	}()
	managers.sell.DeltaChange(v.Price, negStock)
	managers.buy.DeltaChange(v.Price, negStock)

	key = hub.getDealKey(v.TradingPair)
	hub.batch.Set(key, bz)
	hub.sid++
	info = hub.subMan.GetDealSubscribeInfo()
	targets, ok = info[v.TradingPair]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushDeal(target, bz)
	}
}

func (hub *Hub) handleCancelOrderInfo(bz []byte) {
	var v CancelOrderInfo
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal CancelOrderInfo")
		return
	}
	info := hub.subMan.GetOrderSubscribeInfo()
	accAndSeq := strings.Split(v.OrderID, "-")
	if len(accAndSeq) != 2 {
		return
	}
	key := hub.getCancelOrderKey(accAndSeq[0])
	hub.batch.Set(key, bz)
	hub.sid++
	targets, ok := info[accAndSeq[0]]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushCancelOrder(target, bz)
	}

	managers, ok := hub.managersMap[v.TradingPair]
	if !ok {
		return
	}
	negStock := sdk.NewInt(-v.LeftStock)
	hub.depthMutex.Lock()
	defer func() {
		hub.depthMutex.Unlock()
	}()
	if v.Side == market.SELL {
		managers.sell.DeltaChange(v.Price, negStock)
	} else {
		managers.buy.DeltaChange(v.Price, negStock)
	}
}

func (hub *Hub) handleMsgBancorTradeInfoForKafka(bz []byte) {
	var v MsgBancorTradeInfoForKafka
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal MsgBancorTradeInfoForKafka")
		return
	}
	addr := v.Sender.String()
	key := hub.getBancorTradeKey(addr)
	hub.batch.Set(key, bz)
	hub.sid++

	info := hub.subMan.GetBancorTradeSubscribeInfo()
	targets, ok := info[addr]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushBancorTrade(target, bz)
	}
}

func (hub *Hub) handleMsgBancorInfoForKafka(bz []byte) {
	var v MsgBancorInfoForKafka
	err := json.Unmarshal(bz, &v)
	if err != nil {
		hub.Log("Error in Unmarshal MsgBancorInfoForKafka")
		return
	}
	key := hub.getBancorInfoKey(v.Money + "/" + v.Stock)
	hub.batch.Set(key, bz)
	hub.sid++
	info := hub.subMan.GetBancorInfoSubscribeInfo()
	targets, ok := info[v.Stock+"/"+v.Money]
	if !ok {
		return
	}
	for _, target := range targets {
		hub.subMan.PushBancorInfo(target, bz)
	}
}

func (hub *Hub) commitForTicker() {
	tickerMap := make(map[string]*Ticker)
	currMinute := hub.currBlockTime.Hour() * hub.currBlockTime.Minute()
	for _, triman := range hub.managersMap {
		ticker := triman.tman.GetTiker(currMinute)
		if ticker != nil {
			tickerMap[ticker.Market] = ticker
		}
	}
	for _, subscriber := range hub.subMan.GetTickerSubscribeInfo() {
		marketList, ok := subscriber.Detail().([]string)
		if !ok {
			continue
		}
		tickerList := make([]*Ticker, 0, len(marketList))
		for _, market := range marketList {
			ticker, ok := tickerMap[market]
			if ok {
				tickerList = append(tickerList, ticker)
			}
		}
		hub.subMan.PushTicker(subscriber, tickerList)
	}

	hub.tickerMutex.Lock()
	for market, ticker := range tickerMap {
		hub.tickerMap[market] = ticker
	}
	hub.tickerMutex.Unlock()
}

func (hub *Hub) commitForDepth() {
	for market, triman := range hub.managersMap {
		depthDeltaSell := triman.sell.EndBlock()
		depthDeltaBuy := triman.buy.EndBlock()
		if len(depthDeltaSell) == 0 && len(depthDeltaBuy) == 0 {
			continue
		}
		info := hub.subMan.GetDepthSubscribeInfo()
		targets, ok := info[market]
		if !ok {
			continue
		}
		for _, target := range targets {
			if len(depthDeltaSell) != 0 {
				hub.subMan.PushDepthSell(target, depthDeltaSell)
			}
			if len(depthDeltaBuy) != 0 {
				hub.subMan.PushDepthBuy(target, depthDeltaBuy)
			}
		}
	}
}

func (hub *Hub) commit() {
	hub.commitForTicker()
	hub.commitForDepth()
	hub.dbMutex.Lock()
	hub.batch.WriteSync()
	hub.batch = hub.db.NewBatch()
	hub.dbMutex.Unlock()
}

//============================================================

func (hub *Hub) QueryTikers(marketList []string) []*Ticker {
	tickerList := make([]*Ticker, 0, len(marketList))
	hub.tickerMutex.RLock()
	for _, market := range marketList {
		ticker, ok := hub.tickerMap[market]
		if ok {
			tickerList = append(tickerList, ticker)
		}
	}
	hub.tickerMutex.RUnlock()
	return tickerList
}

func (hub *Hub) QueryBlockTime(height int64, count int) []int64 {
	count = limitCount(count)
	data := make([]int64, 0, count)
	end := append([]byte{BlockHeightByte}, int64ToBigEndianBytes(height)...)
	start := []byte{BlockHeightByte}
	hub.dbMutex.RLock()
	iter := hub.db.ReverseIterator(start, end)
	defer func() {
		iter.Close()
		hub.dbMutex.RUnlock()
	}()
	for ; iter.Valid(); iter.Next() {
		unixSec := binary.LittleEndian.Uint64(iter.Value())
		data = append(data, int64(unixSec))
		count--
		if count < 0 {
			break
		}
	}
	return data
}

func (hub *Hub) QueryDepth(market string, count int) (sell []*PricePoint, buy []*PricePoint) {
	count = limitCount(count)
	if !hub.HasMarket(market) {
		return
	}
	tripleMan := hub.managersMap[market]
	hub.depthMutex.RLock()
	sell = tripleMan.sell.GetLowest(count)
	buy = tripleMan.buy.GetHighest(count)
	hub.depthMutex.RUnlock()
	return
}

func (hub *Hub) QueryCandleStick(market string, timespan byte, time int64, sid int64, count int) [][]byte {
	count = limitCount(count)
	data := make([][]byte, 0, count)
	end := getCandleStickEndKey(market, timespan, time, sid)
	start := getCandleStickStartKey(market, timespan)
	hub.dbMutex.RLock()
	iter := hub.db.ReverseIterator(start, end)
	defer func() {
		iter.Close()
		hub.dbMutex.RUnlock()
	}()
	for ; iter.Valid(); iter.Next() {
		data = append(data, iter.Value())
		count--
		if count < 0 {
			break
		}
	}
	return data
}

//=========
func (hub *Hub) QueryOrder(account string, time int64, sid int64, count int) (data [][]byte, tags []byte, timesid []int64) {
	return hub.query(false, OrderByte, []byte(account), time, sid, count)
}

func (hub *Hub) QueryDeal(market string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, DealByte, []byte(market), time, sid, count)
	return
}

func (hub *Hub) QueryBancorInfo(market string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, BancorInfoByte, []byte(market), time, sid, count)
	return
}

func (hub *Hub) QueryBancorTrade(account string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, BancorTradeByte, []byte(account), time, sid, count)
	return
}

func (hub *Hub) QueryRedelegation(account string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, RedelegationByte, []byte(account), time, sid, count)
	return
}
func (hub *Hub) QueryUnbonding(account string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, UnbondingByte, []byte(account), time, sid, count)
	return
}
func (hub *Hub) QueryUnlock(account string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, UnlockByte, []byte(account), time, sid, count)
	return
}

func (hub *Hub) QueryIncome(account string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(true, IncomeByte, []byte(account), time, sid, count)
	return
}

func (hub *Hub) QueryTx(account string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(true, TxByte, []byte(account), time, sid, count)
	return
}

func (hub *Hub) QueryComment(token string, time int64, sid int64, count int) (data [][]byte, timesid []int64) {
	data, _, timesid = hub.query(false, CommentByte, []byte(token), time, sid, count)
	return
}

func (hub *Hub) query(fetchTxDetail bool, firstByte byte, bz []byte, time int64, sid int64,
	count int) (data [][]byte, tags []byte, timesid []int64) {
	count = limitCount(count)
	data = make([][]byte, 0, count)
	tags = make([]byte, 0, count)
	timesid = make([]int64, 0, 2*count)
	start := getStartKeyFromBytes(firstByte, bz)
	end := getEndKeyFromBytes(firstByte, bz, time, sid)
	hub.dbMutex.RLock()
	iter := hub.db.ReverseIterator(start, end)
	defer func() {
		iter.Close()
		hub.dbMutex.RUnlock()
	}()
	for ; iter.Valid(); iter.Next() {
		iKey := iter.Key()
		idx := len(iKey) - 1
		tags = append(tags, iKey[idx])
		sid := binary.BigEndian.Uint64(iKey[idx-8 : idx])
		idx -= 8
		time := binary.BigEndian.Uint64(iKey[idx-8 : idx])
		timesid = append(timesid, []int64{int64(time), int64(sid)}...)
		if fetchTxDetail {
			key := append([]byte{DetailByte}, iter.Value()...)
			data = append(data, hub.db.Get(key))
		} else {
			data = append(data, iter.Value())
		}
		count--
		if count < 0 {
			break
		}
	}
	return
}
