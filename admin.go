package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type AdminStatusResponse struct {
	BlockchainTime int64         `json:"blockchain_time"`
	Strike         float64       `json:"strike"`
	LiveChainlink  float64       `json:"live_chainlink"`
	LiveBinance    float64       `json:"live_binance"`
	MarketTitle    string        `json:"market_title"`
	TokenUp        string        `json:"token_up"`
	TokenDn        string        `json:"token_dn"`
	UpBid          float64       `json:"up_bid"`
	UpAsk          float64       `json:"up_ask"`
	DnBid          float64       `json:"dn_bid"`
	DnAsk          float64       `json:"dn_ask"`
	Wins           int           `json:"wins"`
	Losses         int           `json:"losses"`
	PnL            float64       `json:"pnl"`
	WinRate        float64       `json:"win_rate"`
	Positions      []SimPosition `json:"positions"`
	BTCSpotPrice   float64       `json:"btc_spot_price"`
	ETHPrice       float64       `json:"eth_price"`
}

func StartAdminServer(state *GlobalState, sim *Simulator, port int) {
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		ts := state.BlockchainTime
		strike := state.Strike
		cl := state.LiveChainlink
		bin := state.LiveBinance
		title := state.MarketTitle
		tUp := state.TokenUp
		tDn := state.TokenDn
		uB, uA, dB, dA := state.UpBid, state.UpAsk, state.DnBid, state.DnAsk
		spot := state.BTCSpotPrice
		eth := state.ETHFuturesPrice
		state.mu.RUnlock()

		wins, losses, pnl := sim.GetStats()
		total := wins + losses
		winRate := 0.0
		if total > 0 {
			winRate = (float64(wins) / float64(total)) * 100
		}

		sim.mu.Lock()
		positionsCopy := make([]SimPosition, len(sim.positions))
		copy(positionsCopy, sim.positions)
		sim.mu.Unlock()

		resp := AdminStatusResponse{
			BlockchainTime: ts,
			Strike:         strike,
			LiveChainlink:  cl,
			LiveBinance:    bin,
			MarketTitle:    title,
			TokenUp:        tUp,
			TokenDn:        tDn,
			UpBid:          uB,
			UpAsk:          uA,
			DnBid:          dB,
			DnAsk:          dA,
			Wins:           wins,
			Losses:         losses,
			PnL:            pnl,
			WinRate:        winRate,
			Positions:      positionsCopy,
			BTCSpotPrice:   spot,
			ETHPrice:       eth,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(adminHTML))
	})

	addr := ":" + strconv.Itoa(port)
	go http.ListenAndServe(addr, nil)
}

const adminHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>LiqSniper Admin Panel v1.7.0</title>
    <meta charset="utf-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #334155; padding-bottom: 20px; margin-bottom: 20px; }
        h1 { margin: 0; color: #38bdf8; font-size: 24px; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .card { background: #1e293b; border-radius: 8px; padding: 20px; border: 1px solid #334155; }
        .card h3 { margin: 0 0 10px 0; color: #94a3b8; font-size: 14px; text-transform: uppercase; }
        .card .val { font-size: 24px; font-weight: bold; color: #f1f5f9; }
        .table-container { background: #1e293b; border-radius: 8px; border: 1px solid #334155; overflow: hidden; }
        table { width: 100%; border-collapse: collapse; text-align: left; }
        th, td { padding: 12px 15px; border-bottom: 1px solid #334155; }
        th { background: #0f172a; color: #38bdf8; font-size: 14px; }
        tr:last-child td { border-bottom: none; }
        .badge { padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; }
        .badge-up { background: #15803d; color: #bbf7d0; }
        .badge-dn { background: #b91c1c; color: #fca5a5; }
        .badge-win { background: #166534; color: #4ade80; }
        .badge-loss { background: #991b1b; color: #f87171; }
        .badge-pending { background: #854d0e; color: #fef08a; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🛠 LiqSniper HFT Dashboard & Admin Panel (v1.7.0)</h1>
            <div id="market-title" style="font-weight: bold; color: #e2e8f0;">Loading market...</div>
        </header>
        <div class="grid">
            <div class="card">
                <h3>Time Left (5m)</h3>
                <div class="val" id="time-left">00:00</div>
            </div>
            <div class="card">
                <h3>Live Binance vs Oracle Strike</h3>
                <div class="val" id="prices">$0.00 / $0.00</div>
            </div>
            <div class="card">
                <h3>UP / DN Bids & Asks</h3>
                <div class="val" id="clob-prices">0.0¢ / 0.0¢</div>
            </div>
            <div class="card">
                <h3>Total PnL & WinRate</h3>
                <div class="val" id="pnl" style="color: #4ade80;">$0.00 (0.0%)</div>
            </div>
            <div class="card">
                <h3>BTC Spot / ETH Futures</h3>
                <div class="val" id="extra-prices">$0.00 / $0.00</div>
            </div>
        </div>
        <h2>Positions History</h2>
        <div class="table-container">
            <table>
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Setup Name</th>
                        <th>Direction</th>
                        <th>Strike Price</th>
                        <th>Entry Price</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody id="positions-body">
                    <tr><td colspan="6" style="text-align:center; color:#94a3b8;">No positions triggered yet.</td></tr>
                </tbody>
            </table>
        </div>
    </div>
    <script>
        function formatTime(sec) {
            if (sec <= 0) return "00:00";
            const m = Math.floor(sec / 60); const s = sec % 60;
            return (m < 10 ? "0" : "") + m + ":" + (s < 10 ? "0" : "") + s;
        }
        async function updateStatus() {
            try {
                const res = await fetch('/api/status');
                const data = await res.json();
                document.getElementById('market-title').innerText = data.market_title || "Waiting...";
                const rem = 300 - (data.blockchain_time % 300);
                document.getElementById('time-left').innerText = formatTime(rem);
                document.getElementById('prices').innerHTML = '<span style="color:#e2e8f0;">$' + data.live_binance.toFixed(2) + '</span> / <span style="color:#38bdf8;">$' + data.strike.toFixed(2) + '</span>';
                document.getElementById('clob-prices').innerHTML = '<span style="color:#4ade80;">UP: ' + (data.up_bid*100).toFixed(1) + '¢</span> / <span style="color:#f87171;">DN: ' + (data.dn_bid*100).toFixed(1) + '¢</span>';
                document.getElementById('extra-prices').innerHTML = '<span style="color:#f59e0b;">$' + data.btc_spot_price.toFixed(2) + '</span> / <span style="color:#a855f7;">$' + data.eth_price.toFixed(2) + '</span>';
                
                const pnlColor = data.pnl >= 0 ? '#4ade80' : '#f87171';
                document.getElementById('pnl').style.color = pnlColor;
                document.getElementById('pnl').innerText = '$' + data.pnl.toFixed(3) + ' (' + data.win_rate.toFixed(1) + '%)';

                const tbody = document.getElementById('positions-body');
                if (data.positions && data.positions.length > 0) {
                    let html = '';
                    for (let i = data.positions.length - 1; i >= 0; i--) {
                        const p = data.positions[i];
                        let statusBadge = '';
                        if (!p.Checked) {
                            statusBadge = '<span class="badge badge-pending">PENDING</span>';
                        } else {
                            statusBadge = p.Won ? '<span class="badge badge-win">WIN</span>' : '<span class="badge badge-loss">LOSS</span>';
                        }
                        const dirBadge = p.Direction === 'UP' ? '<span class="badge badge-up">UP</span>' : '<span class="badge badge-dn">DN</span>';
                        html += '<tr>' +
                            '<td>' + p.ID + '</td>' +
                            '<td>' + p.SetupName + '</td>' +
                            '<td>' + dirBadge + '</td>' +
                            '<td>$' + p.StrikePrice.toFixed(2) + '</td>' +
                            '<td>$' + p.EntryPrice.toFixed(2) + '</td>' +
                            '<td>' + statusBadge + '</td>' +
                            '</tr>';
                    }
                    tbody.innerHTML = html;
                } else {
                    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center; color:#94a3b8;">No positions triggered yet.</td></tr>';
                }
            } catch (e) {}
        }
        setInterval(updateStatus, 1000); updateStatus();
    </script>
</body>
</html>
`
