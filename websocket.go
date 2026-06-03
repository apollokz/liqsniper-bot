package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

func WebSocketEngine(state *GlobalState) {
	client := &http.Client{Timeout: 10 * time.Second}
	for {
		state.mu.RLock()
		bcTime := state.BlockchainTime
		state.mu.RUnlock()
		
		if bcTime == 0 {
			time.Sleep(1 * time.Second)
			continue
		}
		
		mStart := bcTime - (bcTime % 300)
		tUp, tDn, title := discoverMarket(client, mStart)
		if tUp == "" {
			time.Sleep(5 * time.Second)
			continue
		}

		state.mu.Lock()
		state.TokenUp, state.TokenDn, state.MarketTitle = tUp, tDn, title
		state.UpBid, state.UpAsk = 0, 0
		state.DnBid, state.DnAsk = 0, 0
		state.mu.Unlock()

		runClobWS(tUp, tDn, mStart, state)
		time.Sleep(1 * time.Second)
	}
}

func discoverMarket(client *http.Client, mStart int64) (string, string, string) {
	url := fmt.Sprintf("https://gamma-api.polymarket.com/events?slug=btc-updown-5m-%d", mStart)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", ""
	}
	defer resp.Body.Close()
	
	var wrapper []struct {
		Title   string `json:"title"`
		Markets []struct {
			ClobTokenIds string `json:"clobTokenIds"`
		} `json:"markets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err == nil && len(wrapper) > 0 && len(wrapper[0].Markets) > 0 {
		var ids []string
		json.Unmarshal([]byte(wrapper[0].Markets[0].ClobTokenIds), &ids)
		if len(ids) >= 2 {
			return ids[0], ids[1], wrapper[0].Title
		}
	}
	return "", "", ""
}

func runClobWS(tUp, tDn string, mStart int64, state *GlobalState) {
	headers := http.Header{"Origin": []string{"https://polymarket.com"}}
	conn, _, err := websocket.DefaultDialer.Dial("wss://ws-subscriptions-clob.polymarket.com/ws/market", headers)
	if err != nil {
		return
	}
	defer conn.Close()

	sub, _ := json.Marshal(map[string]interface{}{
		"type":                   "market",
		"assets_ids":             []string{tUp, tDn},
		"custom_feature_enabled": true,
	})
	conn.WriteMessage(websocket.TextMessage, sub)
	
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			state.mu.RLock()
			currentStart := state.BlockchainTime - (state.BlockchainTime % 300)
			state.mu.RUnlock()
			
			if currentStart > mStart {
				conn.Close()
				return
			}
			_ = conn.WriteMessage(websocket.TextMessage, []byte("PING"))
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var evs []map[string]interface{}
		if err := json.Unmarshal(msg, &evs); err != nil {
			var single map[string]interface{}
			if err := json.Unmarshal(msg, &single); err == nil {
				evs = []map[string]interface{}{single}
			} else {
				continue
			}
		}

		for _, ev := range evs {
			if ev["event_type"] == "best_bid_ask" {
				asset, _ := ev["asset_id"].(string)
				b, _ := strconv.ParseFloat(ev["best_bid"].(string), 64)
				a, _ := strconv.ParseFloat(ev["best_ask"].(string), 64)
				
				state.mu.Lock()
				if asset == tUp {
					state.UpBid, state.UpAsk = b, a
				} else {
					state.DnBid, state.DnAsk = b, a
				}
				state.mu.Unlock()
			}
		}
	}
}
