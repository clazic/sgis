package auth

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// MemoryToken은 consumerKey/consumerSecret 한 쌍 전용 accessToken 메모리 캐시입니다.
//
// CLI의 GetToken(~/.sgis/token.json 파일 캐시)과 달리 파일에 아무것도 쓰지 않으며,
// 한 MemoryToken은 자신의 키 쌍으로 발급한 토큰만 돌려줍니다. 여러 키를 쓰는
// 호출자(MCP 서버의 키 풀 등)는 키마다 MemoryToken을 따로 두어 토큰이 다른 키
// 요청에 섞이지 않게 합니다.
type MemoryToken struct {
	key, secret string
	// AuthURL은 인증 엔드포인트입니다. 비어 있으면 기본 SGIS 인증 URL을 씁니다(테스트용).
	AuthURL string
	// HTTPClient가 nil이면 15초 타임아웃 기본 클라이언트를 씁니다.
	HTTPClient *http.Client

	mu sync.Mutex
	tc *tokenCache
}

// NewMemoryToken은 키 쌍 전용 메모리 토큰 캐시를 만듭니다.
func NewMemoryToken(consumerKey, consumerSecret string) *MemoryToken {
	return &MemoryToken{key: consumerKey, secret: consumerSecret}
}

// Token은 유효한 accessToken을 반환합니다. 캐시가 없거나 만료 임계에 닿으면 재발급합니다.
// 같은 MemoryToken에 대한 동시 호출은 직렬화되어 한 번만 발급합니다.
func (m *MemoryToken) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tc != nil && !needsRefresh(m.tc) {
		return m.tc.AccessToken, nil
	}
	endpoint := m.AuthURL
	if endpoint == "" {
		endpoint = authBaseURL
	}
	client := m.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	tc, err := issueTokenAt(ctx, client, endpoint, m.key, m.secret)
	if err != nil {
		return "", err
	}
	m.tc = tc
	return tc.AccessToken, nil
}

// Invalidate는 캐시된 토큰을 버려 다음 Token 호출이 재발급하게 합니다.
func (m *MemoryToken) Invalidate() {
	m.mu.Lock()
	m.tc = nil
	m.mu.Unlock()
}
