package mcp

import (
	"encoding/json"
	"sync"
	"time"
)

// progressMinInterval: 중간 진행 알림은 이 간격보다 자주 보내지 않습니다(묶기).
var progressMinInterval = time.Second

// progressReporter는 notifications/progress 를 보냅니다.
// 첫 알림(시작)과 마지막 알림(끝)은 항상 보내고, 중간 알림은 1초에 한 번까지만 보냅니다.
type progressReporter struct {
	mu     sync.Mutex
	token  json.RawMessage
	notify func(rpcNotification)
	n      int
	sent   int
	last   time.Time
	done   bool
}

func newProgressReporter(token json.RawMessage, notify func(rpcNotification)) *progressReporter {
	return &progressReporter{token: token, notify: notify}
}

func (p *progressReporter) report(msg string, final bool) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}
	p.n++ // 보내지 않아도 증가 — progress 값은 늘 커짐
	now := time.Now()
	if !final && p.sent > 0 && now.Sub(p.last) < progressMinInterval {
		p.mu.Unlock()
		return
	}
	params := map[string]interface{}{"progressToken": p.token, "progress": p.n}
	if msg != "" {
		params["message"] = msg
	}
	if final {
		params["total"] = p.n
		p.done = true
	}
	p.sent++
	p.last = now
	notify := p.notify
	p.mu.Unlock()
	notify(rpcNotification{JSONRPC: "2.0", Method: "notifications/progress", Params: params})
}
