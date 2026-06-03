package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	// Горутина 1: WebSocket коннект фьючерсов
	go func() {
		for {
			url := "wss://fstream.binance.com/stream?streams=btcusdt@aggtrade/ethusdt@aggtrade/btcusdt@forceorder/btcusdt@depth20@100ms"
			conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				status := "Unknown"
				if resp != nil {
					status = resp.Status
				}
				fmt.Fprintf(os.Stderr, "[BINANCE Futures WS] Сбой прямого коннекта (IP-ban?): %v, HTTP: %s. Переходим на гибридный резервный поток.\n", err, status)
				time.Sleep(5 * time.Second)
				continue
			}

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

				switch strings.ToLower(combined.Stream) {
				case "btcusdt@aggtrade":
					var trade BinanceTradeData
					if json.Unmarshal(combined.Data, &trade) == nil {
						p, _ := strconv.ParseFloat(trade.Price, 64)
						v, _ := strconv.ParseFloat(trade.Volume, 64)
						se.AddFuturesTrade(TradeTick{Timestamp: trade.Timestamp, Price: p, Volume: v, IsBuyerMM: trade.IsBuyerMM})
						
						state.mu.Lock()
						state.LiveBinance = p
						state.BinanceLastUpdate = time.Now().UnixMilli() // Фиксируем живой коннект BTC
						state.mu.Unlock()
					}
				case "ethusdt@aggtrade":
					var trade BinanceTradeData
					if json.Unmarshal(combined.Data, &trade) == nil {
						p, _ := strconv.ParseFloat(trade.Price, 64)
						v, _ := strconv.ParseFloat(trade.Volume, 64)
						se.AddETHTrade(TradeTick{Timestamp: trade.Timestamp, Price: p, Volume: v, IsBuyerMM: trade.IsBuyerMM})
						
						state.mu.Lock()
						state.ETHFuturesPrice = p
						state.ETHLastUpdate = time.Now().UnixMilli() // Фиксируем живой коннект ETH
						state.mu.Unlock()
					}
				case "btcusdt@forceorder":
					var liq BinanceForceOrderData
					if json.Unmarshal(combined.Data, &liq) == nil {
						v, _ := strconv.ParseFloat(liq.Order.Volume, 64)
						p, _ := strconv.ParseFloat(liq.Order.Price, 64)
						se.AddLiquidation(ForceOrderTick{
							Timestamp: time.Now().UnixNano() / 1e6,
							Symbol:    liq.Order.Symbol,
							Side:      liq.Order.Side,
							Volume:    v,
							Price:     p,
						})
					}
				case "btcusdt@depth20@100ms":
					var depth BinanceDepthData
					if json.Unmarshal(combined.Data, &depth) == nil {
						var bids []BookLevel
						var asks []BookLevel
						for i, b := range depth.Bids {
							if i >= 10 { break }
							pr, _ := strconv.ParseFloat(b[0], 64)
							vol, _ := strconv.ParseFloat(b[1], 64)
							bids = append(bids, BookLevel{Price: pr, Volume: vol})
						}
						for i, a := range depth.Asks {
							if i >= 10 { break }
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

	// Горутина 2: REST-поллинг BTC Spot
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		for {
			resp, err := client.Get("https://fapi.binance.com/fapi/v1/ticker/price?symbol=BTCUSDT")
			if err == nil {
				var r struct {
					Price string `json:"price"`
				}
				if json.NewDecoder(resp.Body).Decode(&r) == nil {
					p, _ := strconv.ParseFloat(r.Price, 64)
					if p > 0 {
						state.mu.Lock()
						state.BTCSpotPrice = p
						state.mu.Unlock()
					}
				}
				resp.Body.Close()
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Горутина 3: REST-поллинг ETH Futures (Динамический контроль свежести)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		for {
			time.Sleep(2 * time.Second)
			
			state.mu.RLock()
			lastUpdate := state.ETHLastUpdate
			state.mu.RUnlock()
			
			// Если вебсокет успешно обновил ETH в последние 5 секунд, пропускаем REST-запрос
			if lastUpdate > 0 && (time.Now().UnixMilli() - lastUpdate) < 5000 {
				continue
			}
			
			// Вебсокет молчит — делаем активный REST-запрос
			resp, err := client.Get("https://fapi.binance.com/fapi/v1/ticker/price?symbol=ETHUSDT")
			if err == nil {
				var r struct {
					Price string `json:"price"`
				}
				if json.NewDecoder(resp.Body).Decode(&r) == nil {
					p, _ := strconv.ParseFloat(r.Price, 64)
					if p > 0 {
						state.mu.Lock()
						state.ETHFuturesPrice = p
						state.mu.Unlock()
					}
				}
				resp.Body.Close()
			}
		}
	}()
}
