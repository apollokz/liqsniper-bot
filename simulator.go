package main

import (
	"fmt"
	"sync"
	"time"
)

type SimPosition struct {
	ID          int
	SetupName   string
	Direction   string
	StrikePrice float64
	EntryPrice  float64
	ExpiryTime  int64
	Checked     bool
}

type Simulator struct {
	mu         sync.Mutex
	positions  []SimPosition
	totalWins  int
	totalLoss  int
	netProfit  float64
	counter    int
}

func NewSimulator() *Simulator {
	return &Simulator{
		positions: make([]SimPosition, 0),
	}
}

func (s *Simulator) TriggerTrade(setupName string, direction string, strikePrice float64, currentPrice float64, timeRemaining int64, cvd float64, obi float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	
	// Ограничение: не более одной сделки по конкретному сетапу в рамках одной 5-минутки
	for _, p := range s.positions {
		if p.SetupName == setupName && p.ExpiryTime > now && p.Direction == direction {
			return
		}
	}

	s.counter++
	// Имитация Deep OTM котировок Polymarket в диапазоне $0.05–$0.15
	entryPrice := 0.05 + (float64(s.counter%11) * 0.01) 
	expiryTime := now + timeRemaining

	p := SimPosition{
		ID:          s.counter,
		SetupName:   setupName,
		Direction:   direction,
		StrikePrice: strikePrice,
		EntryPrice:  entryPrice,
		ExpiryTime:  expiryTime,
		Checked:     false,
	}
	s.positions = append(s.positions, p)

	distance := absFloat64(currentPrice - strikePrice)
	// Строгий формат вывода [СИГНАЛ] согласно ТЗ
	fmt.Printf("[СИГНАЛ] TRIGGER: %s. CVD_15s = %.1f BTC. OBI(rho) = %.2f. Ожидаемая цена акций %s = $%.2f. Дистанция до страйка: $%.2f. RR = 1:7.\n",
		setupName, cvd, obi, direction, entryPrice, distance)
}

func (s *Simulator) CheckPositions(currentPrice float64, currentTime int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.positions {
		if !p.Checked && currentTime >= p.ExpiryTime {
			s.positions[i].Checked = true
			win := false
			if p.Direction == "UP" && currentPrice > p.StrikePrice {
				win = true
			} else if p.Direction == "DOWN" && currentPrice < p.StrikePrice {
				win = true
			}

			// Вычитаем Taker fee в 7.2 bps (0.00072$) согласно Red Teaming аудиту
			takerFee := 0.00072

			if win {
				s.totalWins++
				profit := 1.0 - p.EntryPrice - takerFee
				s.netProfit += profit
				fmt.Printf("[РЕЗУЛЬТАТ] УСПЕХ: Акция закрылась по $1.00 (+$%.3f) [ID: %d | Setup: %s | Strike: %.2f | Expiry Price: %.2f]\n",
					profit, p.ID, p.SetupName, p.StrikePrice, currentPrice)
			} else {
				s.totalLoss++
				loss := p.EntryPrice + takerFee
				s.netProfit -= loss
				fmt.Printf("[РЕЗУЛЬТАТ] ПРОВАЛ: Акция сгорела (-$%.3f) [ID: %d | Setup: %s | Strike: %.2f | Expiry Price: %.2f]\n",
					loss, p.ID, p.SetupName, p.StrikePrice, currentPrice)
			}
		}
	}
}

func (s *Simulator) GetStats() (int, int, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalWins, s.totalLoss, s.netProfit
}
