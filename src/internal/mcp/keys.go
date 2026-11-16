package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Cred는 SGIS consumer_key + consumer_secret 한 쌍입니다.
type Cred struct {
	Key    string
	Secret string
}

// id는 토큰 캐시 구분용 해시입니다(키 원문을 맵 키로 쓰지 않음).
func (c Cred) id() string {
	h := sha256.Sum256([]byte(c.Key + "\x00" + c.Secret))
	return hex.EncodeToString(h[:])
}

const maxKeyPartLen = 256

// ParseCred는 "consumer_key:consumer_secret" 한 항목을 읽습니다.
// 오류 메시지에는 키 원문을 넣지 않습니다.
func ParseCred(item string) (Cred, error) {
	item = strings.TrimSpace(item)
	idx := strings.Index(item, ":")
	if idx <= 0 || idx == len(item)-1 {
		return Cred{}, errors.New("키 형식은 consumer_key:consumer_secret 이어야 해요")
	}
	c := Cred{Key: item[:idx], Secret: item[idx+1:]}
	for _, part := range []string{c.Key, c.Secret} {
		if len(part) > maxKeyPartLen || strings.ContainsAny(part, " \t\r\n,") {
			return Cred{}, errors.New("키 형식이 올바르지 않아요(공백·쉼표 금지, 길이 256자 이하)")
		}
	}
	return c, nil
}

// ParseCredList는 문자열 목록을 키 쌍 목록으로 읽습니다.
func ParseCredList(items []string) ([]Cred, error) {
	out := make([]Cred, 0, len(items))
	for i, it := range items {
		c, err := ParseCred(it)
		if err != nil {
			return nil, fmt.Errorf("%d번째 키: %w", i+1, err)
		}
		out = append(out, c)
	}
	return out, nil
}

// ParseEnvKeys는 SGIS_API_KEYS 값(쉼표 구분)을 읽습니다. 빈 항목은 건너뜁니다.
func ParseEnvKeys(v string) ([]Cred, error) {
	var items []string
	for _, p := range strings.Split(v, ",") {
		if strings.TrimSpace(p) != "" {
			items = append(items, p)
		}
	}
	return ParseCredList(items)
}

// secretsOf는 스크럽 대상 문자열(키·시크릿)을 돌려줍니다.
func secretsOf(creds []Cred) []string {
	out := make([]string, 0, 2*len(creds))
	for _, c := range creds {
		out = append(out, c.Key, c.Secret)
	}
	return out
}

// ── 키 풀 ──────────────────────────────────────────────────────────────────

const (
	rateLimitCooldown = 60 * time.Second
	authCooldown      = 10 * time.Minute
)

// keyPool은 라운드로빈 + 쿨다운 키 선택기입니다. 동시 사용에 안전합니다.
type keyPool struct {
	mu       sync.Mutex
	creds    []Cred
	next     int
	cooldown map[int]time.Time
	now      func() time.Time
}

func newKeyPool(creds []Cred) *keyPool {
	return &keyPool{creds: creds, cooldown: map[int]time.Time{}, now: time.Now}
}

// order는 이번 호출에서 시도할 키 순서(인덱스)를 돌려줍니다.
// 쿨다운이 아닌 키를 라운드로빈 순서로 먼저, 쿨다운 중인 키는 풀리는 순서로 뒤에 둡니다.
func (p *keyPool) order() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.creds)
	if n == 0 {
		return nil
	}
	start := p.next % n
	p.next = (p.next + 1) % n
	now := p.now()
	var ready, cooling []int
	for k := 0; k < n; k++ {
		i := (start + k) % n
		if until, ok := p.cooldown[i]; ok && now.Before(until) {
			cooling = append(cooling, i)
		} else {
			ready = append(ready, i)
		}
	}
	sort.SliceStable(cooling, func(a, b int) bool {
		return p.cooldown[cooling[a]].Before(p.cooldown[cooling[b]])
	})
	return append(ready, cooling...)
}

func (p *keyPool) markCooldown(i int, d time.Duration) {
	p.mu.Lock()
	p.cooldown[i] = p.now().Add(d)
	p.mu.Unlock()
}

func (p *keyPool) markOK(i int) {
	p.mu.Lock()
	delete(p.cooldown, i)
	p.mu.Unlock()
}

// ── 스크럽 ─────────────────────────────────────────────────────────────────

// maskSecret은 비밀 문자열을 "***<끝4자리>" 로 바꿉니다(8자 미만이면 "***").
func maskSecret(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return "***" + s[len(s)-4:]
}

// Scrub은 text 안의 비밀 문자열(키·시크릿·accessToken)을 모두 가립니다.
// URL 인코딩된 형태도 함께 찾습니다. 긴 것부터 바꿔 부분 겹침을 피합니다.
func Scrub(text string, secrets []string) string {
	type pair struct{ from, to string }
	var pairs []pair
	seen := map[string]bool{}
	for _, s := range secrets {
		if s == "" {
			continue
		}
		m := maskSecret(s)
		for _, v := range []string{s, url.QueryEscape(s), url.PathEscape(s)} {
			if v != "" && !seen[v] {
				seen[v] = true
				pairs = append(pairs, pair{v, m})
			}
		}
	}
	sort.SliceStable(pairs, func(a, b int) bool { return len(pairs[a].from) > len(pairs[b].from) })
	for _, p := range pairs {
		text = strings.ReplaceAll(text, p.from, p.to)
	}
	return text
}
