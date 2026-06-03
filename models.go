package main

import "sync"

type Tick struct {
	Timestamp int64
	Value     float64
}

type BookLevel struct {
	Price  float64
	Volume float64
}

type GlobalState struct {
	mu             sync.RWMutex
	Strike         float64
	LiveChainlink  float64
	LiveBinance    float64
	UnixTime       int64
	BlockchainTime int64
	MarketTitle    string
	TokenUp, TokenDn string
	UpBid, UpAsk   float64
	DnBid, DnAsk   float64
	LatestBlock    uint64
	Ticks          []Tick

	// Новые высокочастотные поля для Red Team сетапов
	BTCSpotPrice    float64
	ETHFuturesPrice float64
	Bids            []BookLevel
	Asks            []BookLevel
}

type TradeTick struct {
	Timestamp int64 // ms
	Price     float64
	Volume    float64
	IsBuyerMM bool
}

type ForceOrderTick struct {
	Timestamp int64
	Symbol    string
	Side      string
	Volume    float64
	Price     float64
}
