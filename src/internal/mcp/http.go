package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Streamable HTTP(MCP 2025-06-18) 단독 모드. 공식 SDK 대신 직접 구현하고 stdio 와 같은 dispatch·도구 코드를 씁니다.

const (
	maxBodyBytes      = 1 << 20
	maxBatch          = 10
	defaultSessionTTL = time.Hour
	defaultPing       = 15 * time.Second
	headerSession     = "Mcp-Session-Id"
	headerProtocol    = "MCP-Protocol-Version"
	headerAPIKeys     = "X-Api-Keys"
)

// HTTPOptions는 HTTP 모드 설정입니다.
type HTTPOptions struct {
	// Token이 있으면 Authorization: Bearer <Token> 필수(SGIS_MCP_TOKEN).
	Token string
	// AllowedOrigins는 loopback 외에 허용할 Origin 목록(SGIS_MCP_ALLOWED_ORIGINS).
	AllowedOrigins []string
	// SessionTTL: 이 시간 동안 안 쓰인 세션은 버림(기본 1시간).
	SessionTTL time.Duration
	// PingInterval: GET SSE 스트림 연결 유지 주석 간격(기본 15초).
	PingInterval time.Duration
	// AccessLog가 있으면 메서드·도구 이름·걸린 시간만 남김(인자·키·토큰 없음).
	AccessLog *log.Logger
}

// CheckBind는 주소가 loopback 이 아닌데 토큰이 없으면 시작을 거부합니다.
func CheckBind(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("주소 형식 오류(host:port): %v", err)
	}
	if token == "" && !isLoopbackHost(host) {
		return errors.New("loopback(127.0.0.1·::1·localhost)이 아닌 주소는 SGIS_MCP_TOKEN 없이 열 수 없어요")
	}
	return nil
}

func isLoopbackHost(h string) bool {
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// ParseOrigins는 쉼표 구분 Origin 목록을 읽습니다.
func ParseOrigins(v string) []string {
	var out []string
	for _, o := range strings.Split(v, ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			out = append(out, o)
		}
	}
	return out
}

type httpHandler struct {
	s       *Server
	o       HTTPOptions
	tokenH  [32]byte
	allowed map[string]bool
}

// HTTPHandler는 /mcp 하나를 처리하는 Streamable HTTP 핸들러를 만듭니다.
func (s *Server) HTTPHandler(o HTTPOptions) http.Handler {
	if o.SessionTTL == 0 {
		o.SessionTTL = defaultSessionTTL
	}
	if o.PingInterval == 0 {
		o.PingInterval = defaultPing
	}
	h := &httpHandler{s: s, o: o, allowed: map[string]bool{}}
	h.tokenH = sha256.Sum256([]byte(o.Token))
	for _, a := range o.AllowedOrigins {
		h.allowed[strings.TrimRight(a, "/")] = true
	}
	return h
}

// RunHTTP는 addr 에서 HTTP 모드로 서비스합니다. ctx 가 끝나면 정리하고 돌아옵니다.
func (s *Server) RunHTTP(ctx context.Context, addr string, o HTTPOptions) error {
	if err := CheckBind(addr, o.Token); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.HTTPHandler(o), ReadHeaderTimeout: 10 * time.Second}
	ttl := o.SessionTTL
	if ttl == 0 {
		ttl = defaultSessionTTL
	}
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = srv.Shutdown(context.Background())
				s.closeAllSessions()
				return
			case <-t.C:
				s.sweepSessions(ttl)
			}
		}
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// ── 세션 ───────────────────────────────────────────────────────────────────

func newSessionID() string {
	b := make([]byte, 32) // 256bit
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Server) createSession() *session {
	ss := newSession(newSessionID())
	s.sessMu.Lock()
	s.sessions[ss.id] = ss
	s.sessMu.Unlock()
	return ss
}

func (s *Server) lookupSession(id string, ttl time.Duration) *session {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	ss := s.sessions[id]
	if ss == nil {
		return nil
	}
	ss.mu.Lock()
	expired := time.Since(ss.lastUsed) > ttl
	if !expired {
		ss.lastUsed = time.Now()
	}
	ss.mu.Unlock()
	if expired {
		delete(s.sessions, id)
		ss.close()
		return nil
	}
	return ss
}

func (s *Server) deleteSession(id string) bool {
	s.sessMu.Lock()
	ss := s.sessions[id]
	delete(s.sessions, id)
	s.sessMu.Unlock()
	if ss != nil {
		ss.close()
	}
	return ss != nil
}

func (s *Server) sweepSessions(ttl time.Duration) {
	s.sessMu.Lock()
	var dead []*session
	for id, ss := range s.sessions {
		ss.mu.Lock()
		if time.Since(ss.lastUsed) > ttl {
			dead = append(dead, ss)
			delete(s.sessions, id)
		}
		ss.mu.Unlock()
	}
	s.sessMu.Unlock()
	for _, ss := range dead {
		ss.close()
	}
}

func (s *Server) closeAllSessions() {
	s.sessMu.Lock()
	all := s.sessions
	s.sessions = map[string]*session{}
	s.sessMu.Unlock()
	for _, ss := range all {
		ss.close()
	}
}

// ── 공통 검사 ──────────────────────────────────────────────────────────────

func (h *httpHandler) originOK(origin string) bool {
	if origin == "" {
		return true
	}
	o := strings.TrimRight(origin, "/")
	if h.allowed[o] {
		return true
	}
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

func (h *httpHandler) authOK(r *http.Request) bool {
	if h.o.Token == "" {
		return true
	}
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, "Bearer ") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimPrefix(a, "Bearer ")))
	return subtle.ConstantTimeCompare(got[:], h.tokenH[:]) == 1
}

func httpError(w http.ResponseWriter, status int, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(marshalResponse(rpcResponse{Error: &rpcError{Code: code, Message: msg}}))
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if !h.originOK(r.Header.Get("Origin")) {
		httpError(w, http.StatusForbidden, -32000, "허용되지 않은 Origin")
		return
	}
	if !h.authOK(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="sgis-mcp"`)
		httpError(w, http.StatusUnauthorized, -32001, "인증이 필요해요(Authorization: Bearer)")
		return
	}
	if pv := r.Header.Get(headerProtocol); pv != "" && !supportedProtocols[pv] {
		httpError(w, http.StatusBadRequest, -32600, "지원하지 않는 MCP-Protocol-Version")
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.post(w, r)
	case http.MethodGet:
		h.get(w, r)
	case http.MethodDelete:
		sid := r.Header.Get(headerSession)
		if sid == "" {
			httpError(w, http.StatusBadRequest, -32600, "Mcp-Session-Id 머리글이 필요해요")
			return
		}
		if !h.s.deleteSession(sid) {
			httpError(w, http.StatusNotFound, -32001, "세션을 찾을 수 없어요")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		httpError(w, http.StatusMethodNotAllowed, -32600, "허용되지 않은 메서드")
	}
}

// sessionFor는 요청의 세션을 찾습니다. 실패하면 응답을 쓰고 nil.
func (h *httpHandler) sessionFor(w http.ResponseWriter, r *http.Request) *session {
	sid := r.Header.Get(headerSession)
	if sid == "" {
		httpError(w, http.StatusBadRequest, -32600, "Mcp-Session-Id 머리글이 필요해요(먼저 initialize)")
		return nil
	}
	ss := h.s.lookupSession(sid, h.o.SessionTTL)
	if ss == nil {
		httpError(w, http.StatusNotFound, -32001, "세션을 찾을 수 없어요(만료 또는 종료) — 다시 initialize 하세요")
		return nil
	}
	return ss
}

// ── GET: 서버 먼저 알림 SSE ────────────────────────────────────────────────

func (h *httpHandler) get(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		httpError(w, http.StatusNotAcceptable, -32600, "Accept: text/event-stream 이 필요해요")
		return
	}
	ss := h.sessionFor(w, r)
	if ss == nil {
		return
	}
	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if fl != nil {
		fl.Flush()
	}
	t := time.NewTicker(h.o.PingInterval)
	defer t.Stop()
	secrets := secretsOf(h.s.opt.DefaultKeys)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ss.closed:
			return
		case ev := <-ss.events:
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", Scrub(string(ev), secrets))
		case <-t.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			ss.mu.Lock()
			ss.lastUsed = time.Now()
			ss.mu.Unlock()
		}
		if fl != nil {
			fl.Flush()
		}
	}
}

// ── POST ───────────────────────────────────────────────────────────────────

var toolNameRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func parseHeaderKeys(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasProgressToken(params json.RawMessage) bool {
	var p struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(params, &p) != nil {
		return false
	}
	t, ok := p.Meta["progressToken"]
	return ok && len(t) > 0 && string(t) != "null"
}

func (h *httpHandler) post(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		httpError(w, http.StatusRequestEntityTooLarge, -32600, "요청 본문은 1MB 이하여야 해요")
		return
	}
	body = bytes.TrimSpace(body)
	var raws []json.RawMessage
	batch := len(body) > 0 && body[0] == '['
	if batch {
		if err := json.Unmarshal(body, &raws); err != nil {
			httpError(w, http.StatusBadRequest, -32700, "JSON 파싱 오류")
			return
		}
		if len(raws) == 0 || len(raws) > maxBatch {
			httpError(w, http.StatusBadRequest, -32600, fmt.Sprintf("배치는 1~%d개여야 해요", maxBatch))
			return
		}
	} else {
		raws = []json.RawMessage{body}
	}

	msgs := make([]rpcMessage, len(raws))
	parseErr := make([]bool, len(raws))
	hasInit, hasRequest, wantSSE := false, false, false
	for i, raw := range raws {
		if err := json.Unmarshal(raw, &msgs[i]); err != nil || (msgs[i].JSONRPC != "2.0") {
			parseErr[i] = true
			continue
		}
		m := msgs[i]
		if m.Method != "" && !m.isNotification() {
			hasRequest = true
			if m.Method == "initialize" {
				hasInit = true
			}
			if m.Method == "tools/call" && hasProgressToken(m.Params) {
				wantSSE = true
			}
		}
	}
	if !batch && parseErr[0] {
		httpError(w, http.StatusBadRequest, -32700, "JSON-RPC 메시지를 읽을 수 없어요")
		return
	}

	var ss *session
	if hasInit {
		ss = h.s.createSession()
		w.Header().Set(headerSession, ss.id)
	} else if ss = h.sessionFor(w, r); ss == nil {
		return
	}

	env := &callEnv{sess: ss}
	if ks := parseHeaderKeys(r.Header.Get(headerAPIKeys)); len(ks) > 0 {
		env.headerKeys, env.hasHeaderKeys = ks, true
	}
	secrets := secretsOf(h.s.opt.DefaultKeys)
	for _, k := range env.headerKeys {
		secrets = append(secrets, k)
		if i := strings.Index(k, ":"); i >= 0 {
			secrets = append(secrets, k[:i], k[i+1:])
		}
	}
	scrubBytes := func(b []byte) []byte { return []byte(Scrub(string(b), secrets)) }

	// 알림·응답만 담긴 POST → 202
	if !hasRequest {
		for i, m := range msgs {
			if !parseErr[i] {
				h.s.dispatch(r.Context(), m, env)
			}
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	sse := wantSSE && strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var wmu sync.Mutex
	var fl http.Flusher
	if sse {
		fl, _ = w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if fl != nil {
			fl.Flush()
		}
		writeEvent := func(b []byte) {
			wmu.Lock()
			defer wmu.Unlock()
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", scrubBytes(b))
			if fl != nil {
				fl.Flush()
			}
		}
		env.notify = func(n rpcNotification) {
			if b, err := json.Marshal(n); err == nil {
				writeEvent(b)
			}
		}
	}

	responses := make([][]byte, len(msgs))
	var wg sync.WaitGroup
	for i := range msgs {
		if parseErr[i] {
			responses[i] = marshalResponse(rpcResponse{Error: &rpcError{Code: -32600, Message: "잘못된 JSON-RPC 메시지"}})
			continue
		}
		m := msgs[i]
		if m.isNotification() || m.Method == "" {
			h.s.dispatch(r.Context(), m, env)
			continue
		}
		wg.Add(1)
		go func(i int, m rpcMessage) {
			defer wg.Done()
			start := time.Now()
			resp := h.s.dispatch(r.Context(), m, env)
			h.accessLog(m, time.Since(start))
			if resp == nil {
				return
			}
			b := marshalResponse(*resp)
			if sse {
				wmu.Lock()
				_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", scrubBytes(b))
				if fl != nil {
					fl.Flush()
				}
				wmu.Unlock()
				return
			}
			responses[i] = b
		}(i, m)
	}
	wg.Wait()
	if sse {
		return
	}

	var out [][]byte
	for _, b := range responses {
		if b != nil {
			out = append(out, scrubBytes(b))
		}
	}
	if len(out) == 0 {
		w.WriteHeader(http.StatusAccepted) // 모두 취소됨
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if batch {
		_, _ = w.Write(append(append([]byte("["), bytes.Join(out, []byte(","))...), ']'))
		return
	}
	_, _ = w.Write(out[0])
}

func (h *httpHandler) accessLog(m rpcMessage, d time.Duration) {
	if h.o.AccessLog == nil {
		return
	}
	tool := ""
	if m.Method == "tools/call" {
		var p struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(m.Params, &p) == nil && toolNameRe.MatchString(p.Name) {
			tool = " " + p.Name
		} else {
			tool = " ?"
		}
	}
	h.o.AccessLog.Printf("%s%s %dms", truncateRunes(m.Method, 40), tool, d.Milliseconds())
}
