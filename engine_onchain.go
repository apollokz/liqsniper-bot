package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const PolygonRPC = "https://polygon-bor.publicnode.com"

func DataEngine(state *GlobalState) {
	client := &http.Client{Timeout: 3 * time.Second}
	for {
		payload := `{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",false],"id":1}`
		resp, err := client.Post(PolygonRPC, "application/json", strings.NewReader(payload))
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		
		var r struct {
			Result struct {
				Number    string
				Timestamp string
			}
		}
		
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		
		if err != nil || r.Result.Timestamp == "" {
			time.Sleep(1 * time.Second)
			continue
		}

		ts, err := strconv.ParseInt(strings.TrimPrefix(r.Result.Timestamp, "0x"), 16, 64)
		if err != nil || ts == 0 {
			time.Sleep(1 * time.Second)
			continue
		}
		
		block, _ := strconv.ParseUint(strings.TrimPrefix(r.Result.Number, "0x"), 16, 64)

		state.mu.Lock()
		state.BlockchainTime = ts
		state.UnixTime = ts
		state.LatestBlock = block
		state.mu.Unlock()

		time.Sleep(2 * time.Second)
	}
}
