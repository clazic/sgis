// Package relay는 PES 로컬 중계 모드를 담당합니다.
//
// 외부 AI(pes-bridge)가 sgis CLI를 돌릴 때는 CLI가 서비스 ID/보안 Key를 받지 않습니다.
// 환경변수 PES_RELAY_URL이 있으면 중계 모드이며, CLI는 키 대신 자리표시
// (__PES_KEY_i__ / __PES_SECRET_i__)를 쓰고 SGIS로 가는 모든 요청을
// {PES_RELAY_URL}/sgis/<scheme>/<host>/<path>?<query> 로 보냅니다.
// 중계가 실제 키를 끼워 SGIS로 보내고 응답을 그대로 돌려줍니다.
//
// PES_RELAY_URL이 없으면 이 패키지는 아무 동작도 바꾸지 않습니다.
package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	envURL   = "PES_RELAY_URL"
	envToken = "PES_RELAY_TOKEN"
	envKeys  = "PES_RELAY_SGIS_KEYS"

	// toolName은 중계 주소의 <tool> 자리입니다.
	toolName = "sgis"

	headerToken = "X-PES-Relay-Token"
	headerError = "X-PES-Relay-Error"

	// maxErrorBody는 중계 오류 본문을 읽는 한도입니다.
	maxErrorBody = 64 * 1024
)

// NoKeyMessage는 PES에 SGIS 키가 없을 때(키 수 0 또는 변수 없음) 보여 주는 안내입니다.
const NoKeyMessage = "PES 에 SGIS 키가 없어요. PES 「조사 도구 · 그림 키」에서 키를 넣으세요."

// ErrNoKeys는 중계 모드인데 PES에 등록된 SGIS 키 쌍이 없을 때의 오류입니다.
// SGIS를 부르지 않고 바로 끝내야 합니다.
var ErrNoKeys = errors.New(NoKeyMessage)

// Error는 중계가 스스로 낸 오류(토큰 불일치 401 · 허용되지 않은 호스트 403 · 키 없음 409)입니다.
// SGIS 응답이 아니므로 다른 키로 다시 시도하지 않고 Message를 그대로 보여 주고 끝냅니다.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// AsError는 err 사슬에서 중계 오류(*Error)를 찾습니다.
func AsError(err error) (*Error, bool) {
	var re *Error
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}

// IsFatal은 err가 다시 시도해도 소용없는 중계 쪽 오류(키 없음 · 중계 오류)인지 알려 줍니다.
func IsFatal(err error) bool {
	if errors.Is(err, ErrNoKeys) {
		return true
	}
	_, ok := AsError(err)
	return ok
}

// baseURL은 끝의 '/'를 뗀 중계 주소입니다. 중계 모드가 아니면 빈 문자열입니다.
func baseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(envURL)), "/")
}

// Enabled는 중계 모드(PES_RELAY_URL 있음)인지 알려 줍니다.
func Enabled() bool {
	return baseURL() != ""
}

// KeyCount는 PES에 등록된 SGIS 키 쌍 수(PES_RELAY_SGIS_KEYS)입니다.
// 변수가 없거나 0 이상의 정수가 아니면 0입니다.
func KeyCount() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envKeys)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Placeholder는 i번째 쌍의 자리표시(서비스 ID, 보안 Key)를 돌려줍니다.
func Placeholder(i int) (key, secret string) {
	return fmt.Sprintf("__PES_KEY_%d__", i), fmt.Sprintf("__PES_SECRET_%d__", i)
}

// RunID는 이번 실행(중계 주소 + 실행 토큰)을 구분하는 문자열입니다.
// 토큰 캐시 지문에 섞어, 다른 실행에서 받은 accessToken을 다시 쓰지 않게 합니다.
func RunID() string {
	return baseURL() + "\x00" + os.Getenv(envToken)
}

// sgisHostSuffixes는 중계로 보낼 기관(SGIS) 호스트의 도메인 꼬리입니다.
var sgisHostSuffixes = []string{"mods.go.kr", "kostat.go.kr"}

// IsSGISHost는 host(포트 없이)가 SGIS 기관 호스트인지 알려 줍니다.
func IsSGISHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, s := range sgisHostSuffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

// Rewrite는 기관 주소를 중계 주소로 바꿉니다.
//
//	<scheme>://<host>[:port]/<path>?<query>
//	→ {PES_RELAY_URL}/sgis/<scheme>/<host>[:port]/<path>?<query>
func Rewrite(u *url.URL) (*url.URL, error) {
	base, err := url.Parse(baseURL())
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("PES_RELAY_URL 이 올바른 주소가 아니에요")
	}
	prefix := strings.TrimRight(base.EscapedPath(), "/") + "/" + toolName + "/" + u.Scheme + "/" + u.Host
	escaped := prefix + u.EscapedPath()
	path, err := url.PathUnescape(escaped)
	if err != nil {
		return nil, fmt.Errorf("중계 주소 만들기 실패: %w", err)
	}
	out := &url.URL{
		Scheme:   base.Scheme,
		Host:     base.Host,
		Path:     path,
		RawPath:  escaped,
		RawQuery: u.RawQuery,
	}
	return out, nil
}

// transport는 SGIS로 가는 요청을 중계로 돌리는 http.RoundTripper입니다.
type transport struct {
	base http.RoundTripper
}

// Wrap은 중계 모드일 때 base를 중계 전송으로 감쌉니다. 중계 모드가 아니면 base를 그대로 돌려줍니다.
// base가 nil이면 http.DefaultTransport를 씁니다.
//
// 주소 넘김(3xx)은 http.Client가 원래(기관) 주소 기준으로 따라가고, 그 요청이 다시 이 전송을
// 지나므로 Location이 기관 주소면 자동으로 중계 주소로 바뀝니다. 기관이 아닌 호스트로 가는
// 요청은 그대로 보내며 실행 토큰 머리글을 붙이지 않습니다.
func Wrap(base http.RoundTripper) http.RoundTripper {
	if !Enabled() {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !IsSGISHost(req.URL.Hostname()) {
		return t.base.RoundTrip(req)
	}
	ru, err := Rewrite(req.URL)
	if err != nil {
		return nil, err
	}
	// RoundTripper는 받은 요청을 고치면 안 되므로 복사본을 고칩니다.
	r2 := req.Clone(req.Context())
	r2.URL = ru
	r2.Host = ""
	r2.Header.Set(headerToken, os.Getenv(envToken))

	resp, err := t.base.RoundTrip(r2)
	if err != nil {
		return nil, err
	}
	if resp.Header.Get(headerError) == "1" {
		defer resp.Body.Close()
		return nil, readError(resp)
	}
	return resp, nil
}

// readError는 중계 오류 응답({"error":"…"})을 *Error로 읽습니다.
func readError(resp *http.Response) *Error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var payload struct {
		Error string `json:"error"`
	}
	msg := ""
	if json.Unmarshal(body, &payload) == nil {
		msg = strings.TrimSpace(payload.Error)
	}
	if msg == "" {
		msg = fmt.Sprintf("PES 중계 오류 (HTTP %d)", resp.StatusCode)
	}
	return &Error{Status: resp.StatusCode, Message: msg}
}
