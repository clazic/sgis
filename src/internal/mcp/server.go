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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/clazic/sgis/internal/api"
	"github.com/clazic/sgis/internal/auth"
	"github.com/clazic/sgis/internal/config"
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
	// Results는 큰 결과 저장소입니다. nil 이면 환경변수(MCP_RESULTS_DIR 등)·기본 폴더로 만듭니다.
	Results *ResultStore
	// PageBytes는 도구 응답 한 쪽 상한입니다(기본 MaxResultBytes ≈ 200KB, 테스트용).
	PageBytes int
	// Logger는 진단 출력(stderr)입니다. 키를 넣지 않습니다. nil이면 log.Default().
	Logger *log.Logger
}

// Server는 MCP 서버입니다(stdio · Streamable HTTP 공통 도구 코드).
type Server struct {
	opt         Options
	defaultPool *keyPool
	http        *http.Client
	// authHTTP는 accessToken 발급용 클라이언트입니다(요청별 키 사용량을 세는 전송 계층 포함).
	authHTTP   *http.Client
	logger     *log.Logger
	tools      []*Tool
	toolByName map[string]*Tool
	results    *ResultStore
	pageBytes  int

	tokMu  sync.Mutex
	tokens map[string]*auth.MemoryToken

	outMu sync.Mutex
	out   io.Writer

	stdioSess *session

	sessMu   sync.Mutex
	sessions map[string]*session
}

// NewServer는 서버를 만듭니다.
func NewServer(opt Options) *Server {
	s := &Server{
		opt:         opt,
		defaultPool: newKeyPool(opt.DefaultKeys),
		http:        opt.HTTPClient,
		logger:      opt.Logger,
		tokens:      map[string]*auth.MemoryToken{},
		stdioSess:   newSession(""),
		sessions:    map[string]*session{},
	}
	if s.http == nil {
		s.http = api.NewHTTPClient()
	}
	// 요청별 키 사용량(kr.pes/key-usage)을 세도록 전송 계층을 감쌉니다.
	// 받은 클라이언트는 바꾸지 않고 복사본을 씁니다. 집계 대상이 아닌 요청은 그대로 지나갑니다.
	hc := *s.http
	hc.Transport = wrapUsageTransport(hc.Transport)
	s.http = &hc
	s.authHTTP = &http.Client{Timeout: 15 * time.Second, Transport: wrapUsageTransport(nil)}
	if s.logger == nil {
		s.logger = log.Default()
	}
	s.results = opt.Results
	if s.results == nil {
		rs, err := ResultStoreFromEnv()
		if err != nil {
			s.logger.Printf("결과 저장소 설정 오류(%v) — 기본값을 씁니다", err)
			rs = &ResultStore{Dir: filepath.Join(config.ConfigDir(), "cache", "mcp-results"), TTL: defaultResultsTTL, MaxBytes: defaultResultsMaxBytes}
		}
		s.results = rs
	}
	s.pageBytes = opt.PageBytes
	if s.pageBytes <= 0 {
		s.pageBytes = MaxResultBytes
	}
	s.tools = append(buildTools(), resultTools()...)
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

func (m rpcMessage) isNotification() bool {
	return len(m.ID) == 0 || string(m.ID) == "null"
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

type rpcNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

func marshalResponse(resp rpcResponse) []byte {
	resp.JSONRPC = "2.0"
	if resp.ID == nil {
		resp.ID = json.RawMessage("null")
	}
	b, err := json.Marshal(resp)
	if err != nil {
		b, _ = json.Marshal(rpcResponse{JSONRPC: "2.0", ID: resp.ID, Error: &rpcError{Code: -32603, Message: "응답 직렬화 실패"}})
	}
	return b
}

// session은 요청 취소 대상(진행 중 tools/call)을 들고 있는 단위입니다.
// stdio 는 하나, HTTP 는 Mcp-Session-Id 마다 하나.
type session struct {
	id       string
	mu       sync.Mutex
	inflight map[string]context.CancelFunc
	lastUsed time.Time
	events   chan []byte   // 서버가 먼저 보내는 알림(GET 스트림)
	closed   chan struct{} // 세션 종료
	once     sync.Once
}

func newSession(id string) *session {
	return &session{id: id, inflight: map[string]context.CancelFunc{}, lastUsed: time.Now(),
		events: make(chan []byte, 16), closed: make(chan struct{})}
}

func (ss *session) register(id string, cancel context.CancelFunc) {
	ss.mu.Lock()
	ss.inflight[id] = cancel
	ss.mu.Unlock()
}

func (ss *session) unregister(id string) {
	ss.mu.Lock()
	delete(ss.inflight, id)
	ss.mu.Unlock()
}

func (ss *session) cancel(id string) {
	ss.mu.Lock()
	c := ss.inflight[id]
	ss.mu.Unlock()
	if c != nil {
		c()
	}
}

func (ss *session) close() {
	ss.once.Do(func() {
		close(ss.closed)
		ss.mu.Lock()
		for _, c := range ss.inflight {
			c()
		}
		ss.mu.Unlock()
	})
}

// callEnv는 메시지 하나를 처리할 때의 전송 계층 정보입니다.
type callEnv struct {
	sess *session
	// notify는 이 요청에 묶인 알림(진행 등)을 보냅니다. nil 이면 보내지 않음.
	notify func(n rpcNotification)
	// headerKeys는 HTTP X-Api-Keys 머리글 키(있으면 _meta 와 같게 취급).
	headerKeys    []string
	hasHeaderKeys bool
}

// dispatch는 요청 하나를 처리해 응답을 돌려줍니다. 알림이거나 취소된 요청이면 nil.
func (s *Server) dispatch(ctx context.Context, msg rpcMessage, env *callEnv) *rpcResponse {
	if msg.Method == "" {
		if msg.isNotification() && msg.Result == nil && msg.Error == nil {
			return &rpcResponse{Error: &rpcError{Code: -32600, Message: "잘못된 요청"}}
		}
		return nil // 클라이언트가 보낸 응답 — 무시
	}
	if msg.isNotification() {
		s.handleNotification(msg, env)
		return nil
	}
	switch msg.Method {
	case "initialize":
		return &rpcResponse{ID: msg.ID, Result: s.initializeResult(msg.Params)}
	case "ping":
		return &rpcResponse{ID: msg.ID, Result: map[string]interface{}{}}
	case "resources/list":
		return &rpcResponse{ID: msg.ID, Result: map[string]interface{}{"resources": []interface{}{}}}
	case "resources/templates/list":
		return &rpcResponse{ID: msg.ID, Result: map[string]interface{}{"resourceTemplates": []interface{}{}}}
	case "resources/read":
		res, rerr := s.readResource(msg.Params)
		if rerr != nil {
			return &rpcResponse{ID: msg.ID, Error: rerr}
		}
		return &rpcResponse{ID: msg.ID, Result: res}
	case "tools/list":
		return &rpcResponse{ID: msg.ID, Result: map[string]interface{}{"tools": s.toolDescriptors()}}
	case "tools/call":
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()
		key := string(msg.ID)
		env.sess.register(key, cancel)
		defer env.sess.unregister(key)
		res := s.callTool(cctx, msg.Params, env)
		if cctx.Err() != nil {
			return nil // 취소됨(notifications/cancelled · 연결 끊김) — 답하지 않음
		}
		return &rpcResponse{ID: msg.ID, Result: res}
	default:
		return &rpcResponse{ID: msg.ID, Error: &rpcError{Code: -32601, Message: "지원하지 않는 메서드: " + truncateRunes(msg.Method, 64)}}
	}
}

// ── stdio ──────────────────────────────────────────────────────────────────

// Serve는 r에서 줄 단위 JSON-RPC를 읽고 w에 답합니다(stdio 모드). r이 끝나면 진행 중인 호출을 기다린 뒤 돌아옵니다.
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

func (s *Server) writeLine(b []byte) {
	text := Scrub(string(b), secretsOf(s.opt.DefaultKeys))
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = io.WriteString(s.out, text+"\n")
}

func (s *Server) write(resp rpcResponse) { s.writeLine(marshalResponse(resp)) }

func (s *Server) handleLine(ctx context.Context, line []byte, wg *sync.WaitGroup) {
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		s.write(rpcResponse{Error: &rpcError{Code: -32700, Message: "JSON 파싱 오류"}})
		return
	}
	env := &callEnv{sess: s.stdioSess, notify: func(n rpcNotification) {
		b, err := json.Marshal(n)
		if err == nil {
			s.writeLine(b)
		}
	}}
	if msg.Method == "tools/call" && !msg.isNotification() {
		// 동시 처리: 한 요청이 다른 요청을 막지 않음
		wg.Add(1)
		go func() {
			defer wg.Done()
			if resp := s.dispatch(ctx, msg, env); resp != nil {
				s.write(*resp)
			}
		}()
		return
	}
	if resp := s.dispatch(ctx, msg, env); resp != nil {
		s.write(*resp)
	}
}

func (s *Server) handleNotification(msg rpcMessage, env *callEnv) {
	if msg.Method != "notifications/cancelled" {
		return // notifications/initialized 등은 할 일 없음
	}
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(msg.Params, &p) != nil || len(p.RequestID) == 0 {
		return
	}
	env.sess.cancel(string(p.RequestID))
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
			"tools":     map[string]interface{}{"listChanged": false},
			"resources": map[string]interface{}{},
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
	Content []map[string]interface{} `json:"content"`
	IsError bool                     `json:"isError,omitempty"`
	// Meta는 결과 _meta 입니다(요청별 키 사용량 kr.pes/key-usage 등). 비면 내보내지 않습니다.
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

func textResult(text string, isErr bool) toolResult {
	return toolResult{Content: []map[string]interface{}{{"type": "text", "text": text}}, IsError: isErr}
}

// callCtx는 tools/call 한 번의 상태입니다(쓰인 키·토큰 = 스크럽 대상).
type callCtx struct {
	pool *keyPool
	// usage는 요청별 키(_meta · X-Api-Keys)로 처리할 때만 있습니다. 기본 키면 nil(세지 않음).
	usage    *keyUsage
	mu       sync.Mutex
	secrets  []string
	progress *progressReporter
	links    []map[string]interface{}
}

func (c *callCtx) addLink(l map[string]interface{}) {
	c.mu.Lock()
	c.links = append(c.links, l)
	c.mu.Unlock()
}

// report는 진행 알림을 보냅니다(progressToken 이 없으면 아무것도 안 함).
func (c *callCtx) report(msg string) {
	if c != nil && c.progress != nil {
		c.progress.report(msg, false)
	}
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

func (s *Server) callTool(ctx context.Context, raw json.RawMessage, env *callEnv) (res toolResult) {
	cc := &callCtx{}
	cc.secrets = append(cc.secrets, secretsOf(s.opt.DefaultKeys)...)
	for _, it := range env.headerKeys {
		cc.addSecret(it)
		if i := strings.Index(it, ":"); i >= 0 {
			cc.addSecret(it[:i])
			cc.addSecret(it[i+1:])
		}
	}
	defer func() {
		if r := recover(); r != nil {
			res = textResult("내부 오류가 났어요", true)
		}
		for i := range res.Content {
			if t, ok := res.Content[i]["text"].(string); ok {
				res.Content[i]["text"] = Scrub(t, cc.secretList())
			}
		}
		// 요청별 키로 SGIS 를 한 번이라도 불렀으면 키 번호별 사용량을 붙입니다(성공 · 오류 결과 모두).
		// 번호와 횟수만 담고 키 값은 넣지 않습니다. 이미 있는 _meta 와 합칩니다.
		if us := cc.usage.entries(); len(us) > 0 {
			if res.Meta == nil {
				res.Meta = map[string]interface{}{}
			}
			res.Meta[metaKeyUsageField] = us
		}
		if cc.progress != nil && ctx.Err() == nil {
			if res.IsError {
				cc.progress.report("실패", true)
			} else {
				cc.progress.report("완료", true)
			}
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
	if tok, has := p.Meta["progressToken"]; has && env.notify != nil && len(tok) > 0 && string(tok) != "null" {
		cc.progress = newProgressReporter(tok, env.notify)
		cc.progress.report(tool.Name+" 시작", false)
	}

	if tool.NoKeys {
		return s.runTool(ctx, tool, cc, p.Arguments)
	}

	// 키 선택: 요청별 키(_meta 또는 X-Api-Keys 머리글)가 있으면 그것만, 없으면 기본 키.
	rawKeys, has := p.Meta[metaKeysField]
	if !has && env.hasHeaderKeys {
		has = true
		rawKeys, _ = json.Marshal(env.headerKeys)
	}
	if has {
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
		cc.usage = newKeyUsage() // 풀 번호 = 받은 목록 번호(ParseCredList 가 순서를 지킴)
	} else {
		if len(s.opt.DefaultKeys) == 0 {
			return textResult("API 키가 없어요: SGIS_API_KEYS 환경변수(consumer_key:consumer_secret, 쉼표로 여러 개)나 _meta[\"kr.pes/keys\"] 로 키를 넣어 주세요", true)
		}
		cc.pool = s.defaultPool
	}

	return s.runTool(ctx, tool, cc, p.Arguments)
}

func (s *Server) runTool(ctx context.Context, tool *Tool, cc *callCtx, in map[string]interface{}) toolResult {
	args, err := tool.validate(in)
	if err != nil {
		return textResult("입력 오류: "+err.Error(), true)
	}
	text, err := tool.Handler(ctx, s, cc, args)
	if err != nil {
		return textResult(err.Error(), true)
	}
	res := textResult(text, false)
	for _, l := range cc.links {
		res.Content = append(res.Content, l)
	}
	return res
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
	mt.HTTPClient = s.authHTTP
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
				if !mt.Valid() {
					cc.report("accessToken 발급 중")
				}
				t, err := mt.Token(ctx)
				cc.addSecret(t)
				return t, err
			},
			OnTokenExpired: mt.Invalidate,
		})
		// 이 키로 나가는 인증 · 자료 요청을 키 번호 i 로 셉니다(요청별 키일 때만).
		res, err := client.Get(withUsage(ctx, cc.usage, i), path, params)
		if err == nil {
			cc.pool.markOK(i)
			// 저장·응답 전에 원본에서도 키·토큰을 지움(결과 파일에 비밀이 들어가지 않게)
			return json.RawMessage(Scrub(string(res), cc.secretList())), nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch classify(err) {
		case errRateLimit:
			cc.pool.markCooldown(i, rateLimitCooldown)
			cc.report("호출 한도에 걸려 다음 키로 넘어가요")
			continue
		case errAuth:
			mt.Invalidate()
			cc.pool.markCooldown(i, authCooldown)
			cc.report("인증 실패로 다음 키로 넘어가요")
			continue
		}
		return nil, err
	}
	if classify(lastErr) == errAuth {
		// CLI 용 설정 안내(sgis config ...)는 MCP 에 맞지 않아 첫 줄만 남김
		lastErr = errors.New(strings.SplitN(lastErr.Error(), "\n", 2)[0] + " — 키(consumer_key:consumer_secret)를 확인하세요")
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
