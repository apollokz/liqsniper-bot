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

	// Запуск фоновых HFT процессов
	go DataEngine(state)
	go WebSocketEngine(state)
	go RTDSEngine(state)
	StartBinanceStreams(state, se)
	go se.StartEvaluationLoop()

	// Асинхронная проверка результатов симуляции каждую секунду
	go func() {
		for {
			time.Sleep(1 * time.Second)
			state.mu.RLock()
			livePrice := state.LiveBinance
			bcTime := state.BlockchainTime
			state.mu.RUnlock()

			if livePrice > 0 && bcTime > 0 {
				sim.CheckPositions(livePrice, time.Now().Unix())
			}
		}
	}()

	// Интерфейсный цикл обновления статуса в консоли (раз в 10 секунд, без очистки, чтобы не затирать лог сделок)
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		state.mu.RLock()
		ts := state.BlockchainTime
		strike := state.Strike
		liveChainlink := state.LiveChainlink
		liveBinance := state.LiveBinance
		title := state.MarketTitle
		uB, uA, dB, dA := state.UpBid, state.UpAsk, state.DnBid, state.DnAsk
		state.mu.RUnlock()

		if ts == 0 {
			continue
		}
		rem := 300 - (ts % 300)

		wins, loss, pnl := sim.GetStats()
		total := wins + loss
		winRate := 0.0
		if total > 0 {
			winRate = (float64(wins) / float64(total)) * 100
		}

		fmt.Fprintln(os.Stdout, "\n------------------ [STATUS SUMMARY] ------------------")
		fmt.Printf("📊 Market: %s | Strike: $%.2f\n", title, strike)
		fmt.Printf("⏳ Time Left: %02d:%02d | Live Binance: $%.2f | Oracle: $%.2f\n", rem/60, rem%60, liveBinance, liveChainlink)
		fmt.Printf("🟢 UP Bid/Ask: %.1f¢/%.1f¢ | 🔴 DN Bid/Ask: %.1f¢/%.1f¢\n", uB*100, uA*100, dB*100, dA*100)
		fmt.Printf("📈 Paper Trading Stats -> Trades: %d | Wins: %d | Losses: %d | WinRate: %.1f%% | Net PnL: $%.3f\n",
			total, wins, loss, winRate, pnl)
		fmt.Fprintln(os.Stdout, "------------------------------------------------------")
	}
}
