package main

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/gorilla/websocket"
)

type RTDSMessage struct {
	Topic   string `json:"topic"`
	Type    string `json:"type"`
	Payload struct {
		Symbol string  `json:"symbol"`
		Value  float64 `json:"value"`
		Data   []struct {
			Timestamp int64   `json:"timestamp"`
			Value     float64 `json:"value"`
		} `json:"data"`
	} `json:"payload"`
}

func RTDSEngine(state *GlobalState, se *StrategyEngine) {
	for {
		conn, _, err := websocket.DefaultDialer.Dial("wss://ws-live-data.polymarket.com", nil)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		
		sub := `{"action":"subscribe","subscriptions":[{"topic":"crypto_prices_chainlink","type":"*","filters":"{\"symbol\":\"btc/usd\"}"},{"topic":"crypto_prices","type":"*","filters":"{\"symbol\":\"btcusdt\"}"}]}`
		conn.WriteMessage(websocket.TextMessage, []byte(sub))

		go func(c *websocket.Conn) {
			for {
				time.Sleep(5 * time.Second)
				if err := c.WriteMessage(websocket.TextMessage, []byte("PING")); err != nil {
					return
				}
			}
		}(conn)

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				conn.Close()
				break
			}

			var m RTDSMessage
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}

			if m.Type == "subscribe" && m.Topic == "crypto_prices_chainlink" && len(m.Payload.Data) > 0 {
				for _, d := range m.Payload.Data {
					if d.Value > 0 {
						addTick(state, d.Timestamp, d.Value)
					}
				}
				recalculateStrikes(state)
			}

			if m.Type != "update" {
				continue
			}

			if m.Payload.Value > 0 {
				state.mu.Lock()
				if m.Topic == "crypto_prices_chainlink" {
					state.LiveChainlink = m.Payload.Value
				} else if m.Topic == "crypto_prices" {
					state.LiveBinance = m.Payload.Value
					
					// Гибридный инжектор тиков BTC в стратегию при падении прямого соединения
					if time.Now().UnixMilli() - state.BinanceLastUpdate > 2000 {
						state.mu.Unlock()
						se.AddFuturesTrade(TradeTick{
							Timestamp: time.Now().UnixNano() / 1e6,
							Price:     m.Payload.Value,
							Volume:    1.5,
							IsBuyerMM: time.Now().UnixNano()%2 == 0,
						})
						state.mu.Lock()
					}
				}
				state.mu.Unlock()

				if m.Topic == "crypto_prices_chainlink" {
					addTick(state, time.Now().UnixNano()/1e6, m.Payload.Value)
					recalculateStrikes(state)
				}
			}
		}
	}
}

func addTick(state *GlobalState, ts int64, val float64) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, t := range state.Ticks {
		if t.Timestamp == ts {
			return
		}
	}
	state.Ticks = append(state.Ticks, Tick{Timestamp: ts, Value: val})
	sort.Slice(state.Ticks, func(i, j int) bool {
		return state.Ticks[i].Timestamp < state.Ticks[j].Timestamp
	})
	if len(state.Ticks) > 1000 {
		state.Ticks = state.Ticks[len(state.Ticks)-1000:]
	}
}

func recalculateStrikes(state *GlobalState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := state.UnixTime
	if now == 0 {
		now = time.Now().Unix()
	}
	periodStart := (now - (now % 300)) * 1000

	var firstNew float64
	var minNewTs int64 = 9999999999999

	for _, t := range state.Ticks {
		if t.Value <= 0 {
			continue
		}
		if t.Timestamp >= periodStart && t.Timestamp < minNewTs {
			minNewTs = t.Timestamp
			firstNew = t.Value
		}
	}
	if firstNew > 0 {
		state.Strike = firstNew
	}
}
