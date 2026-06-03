package main

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

type BinanceCombinedMsg struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type BinanceTradeData struct {
	Timestamp int64  `json:"T"`
	Price     string `json:"p"`
	Volume    string `json:"q"`
	IsBuyerMM bool   `json:"m"`
}

type BinanceForceOrderData struct {
	Order struct {
		Symbol string `json:"s"`
		Side   string `json:"S"`
		Volume string `json:"q"`
		Price  string `json:"p"`
	} `json:"o"`
}

type BinanceDepthData struct {
	Bids [][]string `json:"b"`
	Asks [][]string `json:"a"`
}

func StartBinanceStreams(state *GlobalState, se *StrategyEngine) {
	// 1. Поток фьючерсов (Combined Streams через wss://fstream.binance.com)
	go func() {
		for {
			url := "wss://fstream.binance.com/stream?streams=btcusdt@aggTrade/ethusdt@aggTrade/btcusdt@forceOrder/btcusdt@depth20@100ms"
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			// Пинг-понг обработчики для удержания коннекта
			conn.SetPingHandler(func(appData string) error {
				_ = conn.WriteMessage(websocket.PongMessage, []byte(appData))
				return nil
			})

			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					conn.Close()
					break
				}

				var combined BinanceCombinedMsg
				if err := json.Unmarshal(msg, &combined); err != nil {
					continue
				}

				switch combined.Stream {
				case "btcusdt@aggTrade":
					var trade BinanceTradeData
					if json.Unmarshal(combined.Data, &trade) == nil {
						p, _ := strconv.ParseFloat(trade.Price, 64)
						v, _ := strconv.ParseFloat(trade.Volume, 64)
						tick := TradeTick{
							Timestamp: trade.Timestamp,
							Price:     p,
							Volume:    v,
							IsBuyerMM: trade.IsBuyerMM,
						}
						se.AddFuturesTrade(tick)
						state.mu.Lock()
						state.LiveBinance = p
						state.mu.Unlock()
					}
				case "ethusdt@aggTrade":
					var trade BinanceTradeData
					if json.Unmarshal(combined.Data, &trade) == nil {
						p, _ := strconv.ParseFloat(trade.Price, 64)
						v, _ := strconv.ParseFloat(trade.Volume, 64)
						tick := TradeTick{
							Timestamp: trade.Timestamp,
							Price:     p,
							Volume:    v,
							IsBuyerMM: trade.IsBuyerMM,
						}
						se.AddETHTrade(tick)
						state.mu.Lock()
						state.ETHFuturesPrice = p
						state.mu.Unlock()
					}
				case "btcusdt@forceOrder":
					var liq BinanceForceOrderData
					if json.Unmarshal(combined.Data, &liq) == nil {
						v, _ := strconv.ParseFloat(liq.Order.Volume, 64)
						p, _ := strconv.ParseFloat(liq.Order.Price, 64)
						tick := ForceOrderTick{
							Timestamp: time.Now().UnixNano() / 1e6,
							Symbol:    liq.Order.Symbol,
							Side:      liq.Order.Side,
							Volume:    v,
							Price:     p,
						}
						se.AddLiquidation(tick)
					}
				case "btcusdt@depth20@100ms":
					var depth BinanceDepthData
					if json.Unmarshal(combined.Data, &depth) == nil {
						var bids []BookLevel
						var asks []BookLevel
						for i, b := range depth.Bids {
							if i >= 10 {
								break
							}
							pr, _ := strconv.ParseFloat(b[0], 64)
							vol, _ := strconv.ParseFloat(b[1], 64)
							bids = append(bids, BookLevel{Price: pr, Volume: vol})
						}
						for i, a := range depth.Asks {
							if i >= 10 {
								break
							}
							pr, _ := strconv.ParseFloat(a[0], 64)
							vol, _ := strconv.ParseFloat(a[1], 64)
							asks = append(asks, BookLevel{Price: pr, Volume: vol})
						}
						state.mu.Lock()
						state.Bids = bids
						state.Asks = asks
						state.mu.Unlock()
					}
				}
			}
		}
	}()

	// 2. Спотовый поток (wss://stream.binance.com)
	go func() {
		for {
			url := "wss://stream.binance.com:9443/ws/btcusdt@aggTrade"
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					conn.Close()
					break
				}

				var trade BinanceTradeData
				if json.Unmarshal(msg, &trade) == nil {
					p, _ := strconv.ParseFloat(trade.Price, 64)
					state.mu.Lock()
					state.BTCSpotPrice = p
					state.mu.Unlock()
				}
			}
		}
	}()
}
