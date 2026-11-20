package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"sync"
)

// metaKeyUsageField는 tools/call 결과 _meta 에 붙는 키별 사용량 칸 이름입니다.
const metaKeyUsageField = "kr.pes/key-usage"

// usageSniffBytes는 errCd 를 읽으려고 응답 본문에서 모아 두는 상한입니다.
// SGIS 오류 봉투는 수백 바이트라 이보다 큰 본문은 성공(자료)으로 봅니다.
const usageSniffBytes = 64 * 1024

// KeyUsage는 요청별 키 하나가 tools/call 한 번 동안 SGIS 에 보낸 HTTP 요청 집계입니다.
// 키 값은 담지 않고, 받은 목록에서의 번호(0부터)만 담습니다.
type KeyUsage struct {
	I       int `json:"i"`
	Calls   int `json:"calls"`
	Errors  int `json:"errors"`
	Limited int `json:"limited"`
}

// keyUsage는 tools/call 한 번의 키별 집계기입니다. 동시 사용에 안전합니다.
type keyUsage struct {
	mu sync.Mutex
	by map[int]*KeyUsage
}

func newKeyUsage() *keyUsage { return &keyUsage{by: map[int]*KeyUsage{}} }

func (u *keyUsage) add(i, calls, errs, limited int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e := u.by[i]
	if e == nil {
		e = &KeyUsage{I: i}
		u.by[i] = e
	}
	e.Calls += calls
	e.Errors += errs
	e.Limited += limited
}

// entries는 요청을 한 번이라도 보낸 키만 번호 순으로 돌려줍니다. 없으면 nil.
func (u *keyUsage) entries() []KeyUsage {
	if u == nil {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]KeyUsage, 0, len(u.by))
	for _, e := range u.by {
		if e.Calls > 0 {
			out = append(out, *e)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(a, b int) bool { return out[a].I < out[b].I })
	return out
}

// usageSlot은 "이 HTTP 요청은 어느 집계기의 몇 번 키 것인가" 입니다(context 로 전달).
type usageSlot struct {
	u *keyUsage
	i int
}

type usageCtxKey struct{}

// withUsage는 ctx 로 나가는 SGIS 요청(인증 · 자료)을 u 의 i번 키로 세게 합니다. u 가 nil 이면 그대로.
func withUsage(ctx context.Context, u *keyUsage, i int) context.Context {
	if u == nil {
		return ctx
	}
	return context.WithValue(ctx, usageCtxKey{}, usageSlot{u: u, i: i})
}

// usageTransport는 요청 context 에 usageSlot 이 있으면 그 요청을 셉니다.
//
//	calls   = 보낸 HTTP 요청
//	errors  = 전송 오류 · HTTP 400 이상 · SGIS errCd != 0 (요청 하나에 한 번만. errCd -100 「자료 없음」은 빼고)
//	limited = HTTP 429 (errors 에 포함)
//
// slot 이 없는 요청(기본 키)은 손대지 않고 그대로 넘깁니다.
type usageTransport struct {
	base http.RoundTripper
}

func wrapUsageTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if _, ok := base.(*usageTransport); ok {
		return base
	}
	return &usageTransport{base: base}
}

func (t *usageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	slot, ok := req.Context().Value(usageCtxKey{}).(usageSlot)
	if !ok {
		return t.base.RoundTrip(req)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil {
		slot.u.add(slot.i, 1, 1, 0)
		return resp, err
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		slot.u.add(slot.i, 1, 1, 1)
	case resp.StatusCode >= 400:
		slot.u.add(slot.i, 1, 1, 0)
	default:
		slot.u.add(slot.i, 1, 0, 0)
		if resp.Body != nil {
			resp.Body = &usageBody{rc: resp.Body, slot: slot}
		}
	}
	return resp, nil
}

// usageBody는 본문을 그대로 흘려보내면서 앞부분만 모아, 다 읽힌 뒤 errCd 를 확인합니다.
// 읽다가 끊기면(전송 오류) 오류로 셉니다. 오류는 요청 하나에 한 번만 더합니다.
type usageBody struct {
	rc   io.ReadCloser
	slot usageSlot
	head []byte
	over bool // 본문이 usageSniffBytes 를 넘음 → errCd 확인 안 함
	done bool
}

func (b *usageBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 && !b.over {
		if len(b.head)+n > usageSniffBytes {
			b.over, b.head = true, nil
		} else {
			b.head = append(b.head, p[:n]...)
		}
	}
	if err == io.EOF {
		b.finish(false)
	} else if err != nil {
		b.finish(true)
	}
	return n, err
}

func (b *usageBody) Close() error {
	// json.Decoder 처럼 EOF 까지 읽지 않고 닫는 경우에도 모은 만큼으로 확인합니다.
	b.finish(false)
	return b.rc.Close()
}

func (b *usageBody) finish(failed bool) {
	if b.done {
		return
	}
	b.done = true
	if !failed && !b.over && len(b.head) > 0 {
		var env struct {
			ErrCd json.Number `json:"errCd"`
		}
		if json.Unmarshal(b.head, &env) == nil {
			// -100 은 「검색 결과가 존재하지 않습니다」(빈 결과) — 키 · 호출 문제가 아니라 오류로 세지 않는다
			if s := env.ErrCd.String(); s != "" && s != "0" && s != "-100" {
				failed = true
			}
		}
	}
	b.head = nil
	if failed {
		b.slot.u.add(b.slot.i, 0, 1, 0)
	}
}
