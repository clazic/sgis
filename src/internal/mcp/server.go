// Package mcp는 sgis-mcp(stdio MCP 서버)의 프로토콜·도구·키 풀 구현입니다.
//
// 기존 CLI의 엔드포인트 레지스트리(api.Registry)·HTTP 클라이언트(api.Client)·
// 토큰 발급(auth)·좌표 재투영(output/geo)을 함수로 직접 씁니다.
// accessToken은 키 쌍별 메모리 캐시(auth.MemoryToken)에만 두고 파일에 쓰지 않습니다.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/clazic/sgis/internal/api"
	"github.com/clazic/sgis/internal/auth"
)

const (
	latestProtocol = "2025-06-18"
	metaKeysField  = "kr.pes/keys"
	maxTokenCache  = 256
)

var supportedProtocols = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

// Options는 서버 설정입니다.
type Options struct {
	Version     string
	DefaultKeys []Cred
	// APIBaseURL / AuthURL 은 테스트용입니다. 비어 있으면 SGIS 공식 호스트를 씁니다.
	APIBaseURL string
	AuthURL    string
	HTTPClient *http.Client
	// Logger는 진단 출력(stderr)입니다. 키를 넣지 않습니다. nil이면 log.Default().
	Logger *log.Logger
}

// Server는 stdio MCP 서버입니다.
type Server struct {
	opt         Options
	defaultPool *keyPool
	http        *http.Client
	logger      *log.Logger
	tools       []*Tool
	toolByName  map[string]*Tool

	tokMu  sync.Mutex
	tokens map[string]*auth.MemoryToken

	outMu sync.Mutex
	out   io.Writer

	reqMu    sync.Mutex
	inflight map[string]context.CancelFunc
}

// NewServer는 서버를 만듭니다.
func NewServer(opt Options) *Server {
	s := &Server{
		opt:         opt,
		defaultPool: newKeyPool(opt.DefaultKeys),
		http:        opt.HTTPClient,
		logger:      opt.Logger,
		tokens:      map[string]*auth.MemoryToken{},
		inflight:    map[string]context.CancelFunc{},
	}
	if s.http == nil {
		s.http = api.NewHTTPClient()
	}
	if s.logger == nil {
		s.logger = log.Default()
	}
	s.tools = buildTools()
	s.toolByName = map[string]*Tool{}
	for _, t := range s.tools {
		s.toolByName[t.Name] = t
	}
	return s
}

// ── JSON-RPC ───────────────────────────────────────────────────────────────

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve는 r에서 줄 단위 JSON-RPC를 읽고 w에 답합니다. r이 끝나면 진행 중인 호출을 기다린 뒤 돌아옵니다.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	s.out = w
	br := bufio.NewReaderSize(r, 1<<20)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		line, err := br.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			s.handleLine(ctx, line, &wg)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (s *Server) write(resp rpcResponse) {
	resp.JSONRPC = "2.0"
	if resp.ID == nil {
		resp.ID = json.RawMessage("null")
	}
	b, err := json.Marshal(resp)
	if err != nil {
		b, _ = json.Marshal(rpcResponse{JSONRPC: "2.0", ID: resp.ID, Error: &rpcError{Code: -32603, Message: "응답 직렬화 실패"}})
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = s.out.Write(append(b, '\n'))
}

func (s *Server) handleLine(ctx context.Context, line []byte, wg *sync.WaitGroup) {
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		s.write(rpcResponse{Error: &rpcError{Code: -32700, Message: "JSON 파싱 오류"}})
		return
	}
	isNotification := len(msg.ID) == 0 || string(msg.ID) == "null"
	if msg.Method == "" {
		// 클라이언트가 보낸 응답(result/error) — 이 서버는 요청을 보내지 않으므로 무시.
		if isNotification && msg.Result == nil && msg.Error == nil {
			s.write(rpcResponse{Error: &rpcError{Code: -32600, Message: "잘못된 요청"}})
		}
		return
	}
	if isNotification {
		s.handleNotification(msg)
		return
	}

	switch msg.Method {
	case "initialize":
		s.write(rpcResponse{ID: msg.ID, Result: s.initializeResult(msg.Params)})
	case "ping":
		s.write(rpcResponse{ID: msg.ID, Result: map[string]interface{}{}})
	case "tools/list":
		s.write(rpcResponse{ID: msg.ID, Result: map[string]interface{}{"tools": s.toolDescriptors()}})
	case "tools/call":
		cctx, cancel := context.WithCancel(ctx)
		key := string(msg.ID)
		s.reqMu.Lock()
		s.inflight[key] = cancel
		s.reqMu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				s.reqMu.Lock()
				delete(s.inflight, key)
				s.reqMu.Unlock()
				cancel()
			}()
			res := s.callTool(cctx, msg.Params)
			if cctx.Err() != nil && ctx.Err() == nil {
				return // 클라이언트가 취소함 — 답하지 않음
			}
			s.write(rpcResponse{ID: msg.ID, Result: res})
		}()
	default:
		s.write(rpcResponse{ID: msg.ID, Error: &rpcError{Code: -32601, Message: "지원하지 않는 메서드: " + truncateRunes(msg.Method, 64)}})
	}
}

func (s *Server) handleNotification(msg rpcMessage) {
	if msg.Method != "notifications/cancelled" {
		return // notifications/initialized 등은 할 일 없음
	}
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(msg.Params, &p) != nil || len(p.RequestID) == 0 {
		return
	}
	s.reqMu.Lock()
	cancel := s.inflight[string(p.RequestID)]
	s.reqMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) initializeResult(params json.RawMessage) map[string]interface{} {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	pv := latestProtocol
	if supportedProtocols[p.ProtocolVersion] {
		pv = p.ProtocolVersion
	}
	return map[string]interface{}{
		"protocolVersion": pv,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{"listChanged": false},
		},
		"serverInfo": map[string]interface{}{
			"name":    "sgis-mcp",
			"title":   "SGIS 통계지리정보서비스",
			"version": s.opt.Version,
		},
		"instructions": "SGIS(통계청 통계지리정보서비스) 읽기 전용 도구. 행정구역 코드는 sgis_admin_codes 로 먼저 찾고, 가용 연도는 sgis_years 로 확인한 뒤 통계를 조회하세요.",
	}
}

func (s *Server) toolDescriptors() []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, map[string]interface{}{
			"name":        t.Name,
			"title":       t.Title,
			"description": t.Description,
			"inputSchema": t.inputSchema(),
			"annotations": map[string]interface{}{
				"readOnlyHint":    true,
				"destructiveHint": false,
				"idempotentHint":  true,
				"openWorldHint":   true,
			},
		})
	}
	return out
}

// ── tools/call ─────────────────────────────────────────────────────────────

type toolResult struct {
	Content []map[string]string `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

func textResult(text string, isErr bool) toolResult {
	return toolResult{Content: []map[string]string{{"type": "text", "text": text}}, IsError: isErr}
}

// callCtx는 tools/call 한 번의 상태입니다(쓰인 키·토큰 = 스크럽 대상).
type callCtx struct {
	pool    *keyPool
	mu      sync.Mutex
	secrets []string
}

func (c *callCtx) addSecret(v string) {
	if v == "" {
		return
	}
	c.mu.Lock()
	c.secrets = append(c.secrets, v)
	c.mu.Unlock()
}

func (c *callCtx) secretList() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.secrets...)
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (res toolResult) {
	cc := &callCtx{}
	cc.secrets = append(cc.secrets, secretsOf(s.opt.DefaultKeys)...)
	defer func() {
		if r := recover(); r != nil {
			res = textResult("내부 오류가 났어요", true)
		}
		for i := range res.Content {
			res.Content[i]["text"] = Scrub(res.Content[i]["text"], cc.secretList())
		}
	}()

	var p struct {
		Name      string                     `json:"name"`
		Arguments map[string]interface{}     `json:"arguments"`
		Meta      map[string]json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return textResult("tools/call 인자를 읽을 수 없어요", true)
	}
	tool, ok := s.toolByName[p.Name]
	if !ok {
		return textResult("알 수 없는 도구: "+truncateRunes(p.Name, 64), true)
	}

	// 키 선택: 요청별 키가 있으면 그것만, 없으면 기본 키.
	if rawKeys, has := p.Meta[metaKeysField]; has {
		var items []string
		if err := json.Unmarshal(rawKeys, &items); err != nil {
			return textResult(`_meta["kr.pes/keys"] 는 문자열 배열이어야 해요`, true)
		}
		for _, it := range items {
			cc.addSecret(strings.TrimSpace(it))
			if i := strings.Index(it, ":"); i >= 0 {
				cc.addSecret(strings.TrimSpace(it[:i]))
				cc.addSecret(strings.TrimSpace(it[i+1:]))
			}
		}
		if len(items) == 0 {
			return textResult(`API 키가 없어요: _meta["kr.pes/keys"] 가 비어 있어요`, true)
		}
		creds, err := ParseCredList(items)
		if err != nil {
			return textResult(`_meta["kr.pes/keys"] 오류: `+err.Error(), true)
		}
		cc.pool = newKeyPool(creds)
	} else {
		if len(s.opt.DefaultKeys) == 0 {
			return textResult("API 키가 없어요: SGIS_API_KEYS 환경변수(consumer_key:consumer_secret, 쉼표로 여러 개)나 _meta[\"kr.pes/keys\"] 로 키를 넣어 주세요", true)
		}
		cc.pool = s.defaultPool
	}

	args, err := tool.validate(p.Arguments)
	if err != nil {
		return textResult("입력 오류: "+err.Error(), true)
	}
	text, err := tool.Handler(ctx, s, cc, args)
	if err != nil {
		return textResult(err.Error(), true)
	}
	return textResult(text, false)
}

// tokenFor는 키 쌍 전용 메모리 토큰 캐시를 돌려줍니다(키마다 분리, 다른 키와 공유 안 함).
func (s *Server) tokenFor(c Cred) *auth.MemoryToken {
	id := c.id()
	s.tokMu.Lock()
	defer s.tokMu.Unlock()
	if mt, ok := s.tokens[id]; ok {
		return mt
	}
	if len(s.tokens) >= maxTokenCache {
		for k := range s.tokens { // 상한 초과 시 아무 하나 버림
			delete(s.tokens, k)
			break
		}
	}
	mt := auth.NewMemoryToken(c.Key, c.Secret)
	mt.AuthURL = s.opt.AuthURL
	s.tokens[id] = mt
	return mt
}

// callAPI는 키 풀 순서대로 SGIS API를 호출합니다. 429·인증 오류면 그 키를 쉬게 하고 다음 키로 넘어갑니다.
func (s *Server) callAPI(ctx context.Context, cc *callCtx, path string, params map[string]string) (json.RawMessage, error) {
	order := cc.pool.order()
	if len(order) == 0 {
		return nil, errors.New("API 키가 없어요")
	}
	var lastErr error
	for _, i := range order {
		cred := cc.pool.creds[i]
		mt := s.tokenFor(cred)
		client := api.NewClientWithOptions(api.ClientOptions{
			BaseURL:    s.opt.APIBaseURL,
			HTTPClient: s.http,
			GetToken: func(ctx context.Context) (string, error) {
				t, err := mt.Token(ctx)
				cc.addSecret(t)
				return t, err
			},
			OnTokenExpired: mt.Invalidate,
		})
		res, err := client.Get(ctx, path, params)
		if err == nil {
			cc.pool.markOK(i)
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch classify(err) {
		case errRateLimit:
			cc.pool.markCooldown(i, rateLimitCooldown)
			continue
		case errAuth:
			mt.Invalidate()
			cc.pool.markCooldown(i, authCooldown)
			continue
		}
		return nil, err
	}
	if len(order) > 1 {
		return nil, fmt.Errorf("모든 키(%d개)가 실패했어요. 마지막 오류: %w", len(order), lastErr)
	}
	return nil, lastErr
}

type errKind int

const (
	errOther errKind = iota
	errRateLimit
	errAuth
)

func classify(err error) errKind {
	if errors.Is(err, auth.ErrInvalidCredential) {
		return errAuth
	}
	msg := err.Error()
	if strings.Contains(msg, "429") {
		return errRateLimit
	}
	if strings.Contains(msg, "errCd=-401") {
		return errAuth
	}
	return errOther
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
