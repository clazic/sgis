package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/clazic/sgis/internal/auth"
	"github.com/clazic/sgis/internal/relay"
)

const (
	apiBaseURL          = "https://sgisapi.mods.go.kr/OpenAPI3/"
	httpClientTimeout   = 30 * time.Second
	maxIdleConnsPerHost = 16
	idleConnTimeout     = 90 * time.Second
	maxRetries          = 3
	initialBackoff      = 1 * time.Second
)

// envelope는 SGIS API의 공통 응답 봉투 구조체입니다.
// GeoJSON 엔드포인트는 이 봉투 없이 FeatureCollection을 직접 반환합니다.
type envelope struct {
	Result json.RawMessage `json:"result"`
	ErrCd  int             `json:"errCd"`
	ErrMsg string          `json:"errMsg"`
	ID     string          `json:"id"`
	TrID   string          `json:"trId"`
}

// tokenProvider는 accessToken을 반환하는 함수 타입입니다.
// 테스트에서 auth.GetToken 대신 주입할 수 있습니다.
type tokenProvider func(ctx context.Context) (string, error)

// Client는 SGIS Open API HTTP 클라이언트입니다.
type Client struct {
	baseURL    string
	httpClient *http.Client
	getToken   tokenProvider
	// onTokenExpired는 토큰 만료 errCd를 받았을 때 재발급 전에 호출됩니다(선택).
	onTokenExpired func()
	// relayPairs > 0 이면 PES 중계 모드입니다. 자리표시 쌍 relayPairs개를 풀로 씁니다.
	relayPairs int
}

// ClientOptions는 NewClientWithOptions 설정입니다. CLI는 NewClient를 그대로 씁니다.
type ClientOptions struct {
	// BaseURL이 비어 있으면 기본 SGIS OpenAPI3 URL을 씁니다. 끝에 '/'가 있어야 합니다.
	BaseURL string
	// GetToken은 accessToken 공급자입니다(필수).
	GetToken func(ctx context.Context) (string, error)
	// OnTokenExpired는 토큰 만료 응답 시 재발급 전에 불립니다(캐시 무효화 등, 선택).
	OnTokenExpired func()
	// HTTPClient가 nil이면 기본 전송 설정의 클라이언트를 만듭니다.
	HTTPClient *http.Client
}

// NewHTTPClient는 SGIS API 호출용 기본 HTTP 클라이언트를 만듭니다.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: httpClientTimeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			IdleConnTimeout:     idleConnTimeout,
			ForceAttemptHTTP2:   true,
		},
	}
}

// NewClientWithOptions는 토큰 공급자·기본 URL을 직접 지정한 클라이언트를 만듭니다.
// 파일 토큰 캐시(auth.GetToken)를 쓰지 않는 호출자(MCP 서버 키 풀 등)용입니다.
func NewClientWithOptions(o ClientOptions) *Client {
	c := &Client{
		baseURL:        o.BaseURL,
		httpClient:     o.HTTPClient,
		getToken:       o.GetToken,
		onTokenExpired: o.OnTokenExpired,
	}
	if c.baseURL == "" {
		c.baseURL = apiBaseURL
	}
	if c.httpClient == nil {
		c.httpClient = NewHTTPClient()
	}
	return c
}

// NewClient는 기본 설정으로 SGIS API 클라이언트를 생성합니다.
// 인증은 internal/auth.GetToken 을 통해 자동으로 처리됩니다.
//
// PES 중계 모드(PES_RELAY_URL)에서는 모든 요청을 중계로 보내고, 설정 파일의 키 대신
// 자리표시 쌍을 씁니다. PES에 키가 없으면 SGIS를 부르지 않고 relay.ErrNoKeys 를 돌려줍니다.
func NewClient() (*Client, error) {
	c := &Client{
		baseURL:  apiBaseURL,
		getToken: auth.GetToken,
		httpClient: &http.Client{
			Timeout: httpClientTimeout,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConnsPerHost: maxIdleConnsPerHost,
				IdleConnTimeout:     idleConnTimeout,
				ForceAttemptHTTP2:   true,
			},
		},
	}
	if relay.Enabled() {
		n := relay.KeyCount()
		if n == 0 {
			return nil, relay.ErrNoKeys
		}
		c.relayPairs = n
		c.httpClient.Transport = relay.Wrap(c.httpClient.Transport)
	}
	return c, nil
}

// relayStart는 중계 모드 키 풀에서 이번 실행이 먼저 쓸 쌍 번호를 고릅니다.
// CLI는 한 번 돌고 끝나므로, 여러 실행이 쌍을 고르게 나눠 쓰도록 무작위로 시작합니다.
// 테스트에서 바꿀 수 있습니다.
var relayStart = func(n int) int { return rand.Intn(n) }

// getViaRelayPool은 중계 모드에서 자리표시 쌍을 차례로 써서 호출합니다.
// 429(재시도 소진)·인증 거부면 다음 쌍으로 넘어갑니다. 중계가 스스로 낸 오류는
// 다른 쌍으로 다시 해 보지 않고 바로 돌려줍니다.
func (c *Client) getViaRelayPool(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	n := c.relayPairs
	start := 0
	if n > 1 {
		start = relayStart(n) % n
	}
	var lastErr error
	for k := 0; k < n; k++ {
		i := (start + k) % n
		sub := *c
		sub.relayPairs = 0
		sub.getToken = func(ctx context.Context) (string, error) { return auth.GetRelayToken(ctx, i) }
		sub.onTokenExpired = func() { auth.InvalidateRelayToken(i) }

		res, err := sub.Get(ctx, path, params)
		if err == nil {
			return res, nil
		}
		if relay.IsFatal(err) {
			return nil, relayCause(err)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		if !isKeyFailure(err) {
			return nil, err
		}
	}
	if n > 1 {
		return nil, fmt.Errorf("모든 키(%d개)가 실패했어요. 마지막 오류: %w", n, lastErr)
	}
	return nil, lastErr
}

// relayCause는 err 사슬에서 중계 오류만 꺼냅니다(주소가 붙은 *url.Error 겉을 벗김).
func relayCause(err error) error {
	if re, ok := relay.AsError(err); ok {
		return re
	}
	if errors.Is(err, relay.ErrNoKeys) {
		return relay.ErrNoKeys
	}
	return err
}

// isKeyFailure는 다른 키로 바꾸면 풀릴 수 있는 오류(호출 한도 · 인증 거부)인지 알려 줍니다.
func isKeyFailure(err error) bool {
	if errors.Is(err, auth.ErrInvalidCredential) || errors.Is(err, errRateLimited) {
		return true
	}
	return strings.Contains(err.Error(), "errCd=-401")
}

// errRateLimited는 429 재시도를 모두 쓰고도 실패한 경우입니다.
var errRateLimited = errors.New("SGIS API 429 Too Many Requests")

// maskToken은 에러 문자열에 포함된 토큰을 마스킹합니다.
func maskToken(s, token string) string {
	if token == "" || len(token) < 8 {
		return s
	}
	masked := token[:4] + "****"
	return strings.ReplaceAll(s, token, masked)
}

// Get은 path와 params로 SGIS API를 호출하고 result 필드의 raw JSON을 반환합니다.
// accessToken은 자동으로 주입됩니다.
// GeoJSON 엔드포인트(.geojson)는 봉투 없이 FeatureCollection을 직접 반환하므로
// 전체 응답 body를 그대로 반환합니다.
// 429 응답은 exponential backoff(1s/2s/4s, 최대 3회)로 재시도합니다.
// 토큰 만료(errCd 지시)가 감지되면 1회 강제 재발급 후 재시도합니다.
func (c *Client) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	if c.relayPairs > 0 {
		return c.getViaRelayPool(ctx, path, params)
	}
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("accessToken 획득 실패: %w", err)
	}
	return c.doGet(ctx, path, params, token, false)
}

// doGet은 실제 HTTP 요청을 수행합니다.
// forceRefresh=true면 토큰 재발급 후 재시도하는 경로입니다 (1회만 허용).
func (c *Client) doGet(ctx context.Context, path string, params map[string]string, token string, forceRefresh bool) (json.RawMessage, error) {
	isGeoJSON := strings.HasSuffix(path, ".geojson")

	// URL 조합
	rawURL := c.baseURL + path
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("URL 파싱 실패 (%s): %w", rawURL, err)
	}
	q := u.Query()
	q.Set("accessToken", token)
	for k, v := range params {
		if k != "accessToken" { // 중복 방지
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	// 429 backoff 재시도 루프
	backoff := initialBackoff
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("요청 생성 실패: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP 요청 실패: %w", err)
			// 네트워크 오류는 재시도하지 않음
			return nil, lastErr
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = fmt.Errorf("%w (재시도 %d/%d)", errRateLimited, attempt+1, maxRetries)
			continue
		}

		body, err := func() ([]byte, error) {
			defer resp.Body.Close()
			var buf []byte
			tmp := make([]byte, 32*1024)
			for {
				n, rerr := resp.Body.Read(tmp)
				if n > 0 {
					buf = append(buf, tmp[:n]...)
				}
				if rerr != nil {
					if rerr.Error() == "EOF" {
						return buf, nil
					}
					return buf, rerr
				}
			}
		}()
		if err != nil {
			return nil, fmt.Errorf("응답 body 읽기 실패: %w", err)
		}

		// GeoJSON 엔드포인트: 봉투 없이 직접 반환
		if isGeoJSON {
			return json.RawMessage(body), nil
		}

		// 일반 JSON: 봉투 파싱
		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			// 응답이 봉투 형식이 아닐 수 있음 — raw 반환 시도
			return json.RawMessage(body), nil
		}

		if env.ErrCd != 0 {
			errMsg := maskToken(env.ErrMsg, token)
			// 토큰 만료/무효 오류: 1회 강제 재발급 후 재시도
			if !forceRefresh && isTokenExpiredError(env.ErrCd) {
				if c.onTokenExpired != nil {
					c.onTokenExpired()
				}
				newToken, tErr := c.getToken(ctx)
				if tErr != nil {
					return nil, fmt.Errorf("토큰 재발급 실패: %w", tErr)
				}
				// 새 토큰으로 URL 재구성
				q2 := u.Query()
				q2.Set("accessToken", newToken)
				u.RawQuery = q2.Encode()
				return c.doGet(ctx, path, params, newToken, true)
			}
			return nil, fmt.Errorf("SGIS API 오류 (errCd=%d): %s", env.ErrCd, errMsg)
		}

		return env.Result, nil
	}

	return nil, lastErr
}

// isTokenExpiredError는 errCd가 토큰 만료/무효를 나타내는지 판단합니다.
// SGIS 문서에서 확정된 토큰 관련 에러 코드입니다.
func isTokenExpiredError(errCd int) bool {
	// SGIS 토큰 관련 에러 코드: -401(인증 실패), -100(토큰 없음/만료)
	switch errCd {
	case -401, -100:
		return true
	}
	return false
}

// Call은 Endpoint를 사용해 API를 호출하는 편의 메서드입니다.
// ep.Path를 경로로, params를 쿼리 파라미터로 사용합니다.
func (c *Client) Call(ctx context.Context, ep *Endpoint, params map[string]string) (json.RawMessage, error) {
	if ep == nil {
		return nil, fmt.Errorf("엔드포인트가 nil입니다")
	}
	return c.Get(ctx, ep.Path, params)
}
