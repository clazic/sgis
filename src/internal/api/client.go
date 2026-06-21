package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/clazic/sgis/internal/auth"
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
}

// NewClient는 기본 설정으로 SGIS API 클라이언트를 생성합니다.
// 인증은 internal/auth.GetToken 을 통해 자동으로 처리됩니다.
func NewClient() (*Client, error) {
	return &Client{
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
	}, nil
}

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
			lastErr = fmt.Errorf("SGIS API 429 Too Many Requests (재시도 %d/%d)", attempt+1, maxRetries)
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
