package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	state := &GlobalState{}
	sim := NewSimulator()
	se := NewStrategyEngine(state, sim)

	go DataEngine(state)
	go WebSocketEngine(state)
	go RTDSEngine(state)
	StartBinanceStreams(state, se)
	go se.StartEvaluationLoop()

	StartAdminServer(state, sim, 39182)

	go func() {
		for {
			time.Sleep(1 * time.Second)
			state.mu.RLock()
			livePrice := state.LiveBinance
			bcTime := state.BlockchainTime
			state.mu.RUnlock()
			if livePrice > 0 && bcTime > 0 { sim.CheckPositions(livePrice, time.Now().Unix()) }
		}
	}()

	for {
		time.Sleep(10 * time.Second)
		state.mu.RLock()
		ts := state.BlockchainTime
		strike := state.Strike
		liveChainlink := state.LiveChainlink
		liveBinance := state.LiveBinance
		uB, uA, dB, dA := state.UpBid, state.UpAsk, state.DnBid, state.DnAsk
		state.mu.RUnlock()

		if ts == 0 { continue }
		rem := 300 - (ts % 300)

		wins, loss, pnl := sim.GetStats()
		total := wins + loss
		winRate := 0.0
		if total > 0 {
			winRate = (float64(wins) / float64(total)) * 100
		}

		fmt.Fprintf(os.Stdout, "[ДАШБОРД v1.7.0] СТРАЙК: $%.2f | БИНАНС: $%.2f | ОРАКУЛ: $%.2f | ВРЕМЯ: %02d:%02d\n",
			strike, liveBinance, liveChainlink, rem/60, rem%60)
		fmt.Fprintf(os.Stdout, "[ДАШБОРД v1.7.0] Сделки: %d | Победы: %d | Поражения: %d | WinRate: %.1f%% | PnL: $%.3f\n",
			total, wins, loss, winRate, pnl)
		fmt.Fprintf(os.Stdout, "[ДАШБОРД v1.7.0] UP Bid/Ask: %.1f¢/%.1f¢ | DN Bid/Ask: %.1f¢/%.1f¢\n", uB*100, uA*100, dB*100, dA*100)
	}
}
