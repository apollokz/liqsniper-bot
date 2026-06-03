package main

import (
	"sync"
	"time"
)

type StrategyEngine struct {
	state         *GlobalState
	futuresTrades []TradeTick
	ethTrades     []TradeTick
	liquidations  []ForceOrderTick
	mu            sync.Mutex
	sim           *Simulator
}

func NewStrategyEngine(state *GlobalState, sim *Simulator) *StrategyEngine {
	return &StrategyEngine{state: state, sim: sim}
}

func (se *StrategyEngine) AddFuturesTrade(t TradeTick) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.futuresTrades = append(se.futuresTrades, t)
	se.prune()
}

func (se *StrategyEngine) AddETHTrade(t TradeTick) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.ethTrades = append(se.ethTrades, t)
	se.prune()
}

func (se *StrategyEngine) AddLiquidation(l ForceOrderTick) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.liquidations = append(se.liquidations, l)
}

func (se *StrategyEngine) prune() {
	nowMs := time.Now().UnixNano() / 1e6
	cutoff := nowMs - 45000

	var activeFut []TradeTick
	for _, t := range se.futuresTrades {
		if t.Timestamp >= cutoff { activeFut = append(activeFut, t) }
	}
	se.futuresTrades = activeFut

	var activeEth []TradeTick
	for _, t := range se.ethTrades {
		if t.Timestamp >= cutoff { activeEth = append(activeEth, t) }
	}
	se.ethTrades = activeEth
}

func (se *StrategyEngine) CalculateCVD15s() float64 {
	se.mu.Lock()
	defer se.mu.Unlock()
	nowMs := time.Now().UnixNano() / 1e6
	cutoff := nowMs - 15000
	var cvd float64
	for _, t := range se.futuresTrades {
		if t.Timestamp >= cutoff {
			if t.IsBuyerMM { cvd -= t.Volume } else { cvd += t.Volume }
		}
	}
	return cvd
}

func (se *StrategyEngine) CalculateOBI() float64 {
	se.state.mu.RLock()
	defer se.state.mu.RUnlock()

	if len(se.state.Bids) > 0 && len(se.state.Asks) > 0 {
		topBid := se.state.Bids[0].Price
		topAsk := se.state.Asks[0].Price
		boundaryBid := topBid * (1 - 0.005)
		boundaryAsk := topAsk * (1 + 0.005)

		var bidVol float64
		var askVol float64

		for _, b := range se.state.Bids {
			if b.Price >= boundaryBid { bidVol += b.Volume }
		}
		for _, a := range se.state.Asks {
			if a.Price <= boundaryAsk { askVol += a.Volume }
		}

		total := bidVol + askVol
		if total == 0 { return 0 }
		return (bidVol - askVol) / total
	}

	if se.state.UpBid > 0 && se.state.DnBid > 0 {
		total := se.state.UpBid + se.state.DnBid
		if total == 0 { return 0 }
		return (se.state.UpBid - se.state.DnBid) / total
	}

	return 0
}

func (se *StrategyEngine) GetTradeIntensity() (float64, float64) {
	se.mu.Lock()
	defer se.mu.Unlock()
	nowMs := time.Now().UnixNano() / 1e6
	cutoff1s := nowMs - 1000
	cutoff30s := nowMs - 30000

	var count1s float64
	var count30s float64

	for _, t := range se.futuresTrades {
		if t.Timestamp >= cutoff1s { count1s++ }
		if t.Timestamp >= cutoff30s { count30s++ }
	}
	avg1s := count30s / 30.0
	return count1s, avg1s
}

// ФИКС v2.2.1: Ищем последнюю цену ДО начала 1s-окна (итерация с конца)
// Старый баг: брал первую цену ВНУТРИ окна -> priceNow == price1sAgo -> delta=0
func (se *StrategyEngine) GetPriceVelocity1s() float64 {
	se.mu.Lock()
	defer se.mu.Unlock()

	if len(se.futuresTrades) < 2 {
		return 0
	}

	nowMs := time.Now().UnixNano() / 1e6
	cutoff1s := nowMs - 1000

	priceNow := se.futuresTrades[len(se.futuresTrades)-1].Price

	// Идём с конца к началу — ищем последний тик СТРОГО ДО окна 1s
	price1sAgo := se.futuresTrades[0].Price // fallback: самый старый тик в буфере
	found := false
	for i := len(se.futuresTrades) - 1; i >= 0; i-- {
		if se.futuresTrades[i].Timestamp < cutoff1s {
			price1sAgo = se.futuresTrades[i].Price
			found = true
			break
		}
	}

	// Если все тики моложе 1s — буфер слишком свежий, берём самый старый как референс
	if !found {
		price1sAgo = se.futuresTrades[0].Price
	}

	if price1sAgo == 0 {
		return 0
	}
	return (priceNow - price1sAgo) / price1sAgo * 100
}

func (se *StrategyEngine) GetLocal5mHighLow(periodStart int64) (float64, float64) {
	se.state.mu.RLock()
	defer se.state.mu.RUnlock()

	var high float64 = 0
	var low float64 = 99999999

	for _, t := range se.state.Ticks {
		if t.Timestamp >= periodStart {
			if t.Value > high { high = t.Value }
			if t.Value < low { low = t.Value }
		}
	}
	return high, low
}

func (se *StrategyEngine) StartEvaluationLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	for range ticker.C {
		se.state.mu.RLock()
		bcTime := se.state.BlockchainTime
		strike := se.state.Strike
		livePrice := se.state.LiveBinance
		spotPrice := se.state.BTCSpotPrice
		ethPrice := se.state.ETHFuturesPrice
		se.state.mu.RUnlock()

		if bcTime == 0 || strike == 0 || livePrice == 0 { continue }

		rem := 300 - (bcTime % 300)
		if rem < 90 || rem > 180 { continue }

		dist := absFloat64(livePrice - strike)
		if dist > 80.0 { continue }

		cvd15s := se.CalculateCVD15s()
		obi := se.CalculateOBI()
		v1s := se.GetPriceVelocity1s()
		intensity1s, avgIntensity := se.GetTradeIntensity()

		if absFloat64(v1s) > 0.015 {
			direction := "DOWN"
			if v1s > 0 { direction = "UP" }
			se.sim.TriggerTrade("Latency Arbitrage", direction, strike, livePrice, rem, cvd15s, obi)
		}

		if cvd15s < -5.0 && obi > 0.55 {
			se.sim.TriggerTrade("Limit Absorption", "UP", strike, livePrice, rem, cvd15s, obi)
		} else if cvd15s > 5.0 && obi < -0.55 {
			se.sim.TriggerTrade("Limit Absorption", "DOWN", strike, livePrice, rem, cvd15s, obi)
		}

		if intensity1s > 2.5*avgIntensity && avgIntensity > 5 {
			periodStart := (bcTime - (bcTime % 300)) * 1000
			high5m, low5m := se.GetLocal5mHighLow(periodStart)
			if livePrice < low5m && cvd15s > 0 {
				se.sim.TriggerTrade("Stop Run Sweep", "UP", strike, livePrice, rem, cvd15s, obi)
			} else if livePrice > high5m && cvd15s < 0 {
				se.sim.TriggerTrade("Stop Run Sweep", "DOWN", strike, livePrice, rem, cvd15s, obi)
			}
		}

		if ethPrice > 0 && spotPrice > 0 {
			ethPct := (ethPrice - spotPrice) / spotPrice * 100
			if ethPct > 0.15 {
				se.sim.TriggerTrade("Cross-Asset ETH Lead", "UP", strike, livePrice, rem, cvd15s, obi)
			} else if ethPct < -0.15 {
				se.sim.TriggerTrade("Cross-Asset ETH Lead", "DOWN", strike, livePrice, rem, cvd15s, obi)
			}
		}
	}
}

func absFloat64(x float64) float64 {
	if x < 0 { return -x }
	return x
}

func absInt64(x int64) int64 {
	if x < 0 { return -x }
	return x
}
