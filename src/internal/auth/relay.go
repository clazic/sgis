package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/relay"
)

// PES 중계 모드의 accessToken 캐시.
//
// 중계 모드에서도 CLI는 평소처럼 인증 엔드포인트를 부릅니다. 다만 서비스 ID/보안 Key 자리에
// 자리표시(__PES_KEY_i__ / __PES_SECRET_i__)를 넣어 중계로 보내고, 중계가 실제 쌍으로 바꿔
// SGIS에 보냅니다. 돌려받은 accessToken은 보통 토큰과 똑같이 씁니다.
//
// 캐시는 사용자 자신의 토큰(token.json)과 섞이지 않도록 쌍마다 token.relay.<i>.json 에 따로 두고,
// 지문에 이번 실행(중계 주소 + 실행 토큰)을 섞어 다른 실행이 남긴 토큰은 다시 쓰지 않습니다.

// relayTokenPath는 i번째 쌍의 중계 모드 토큰 캐시 경로입니다.
func relayTokenPath(i int) string {
	return filepath.Join(configDirFunc(), fmt.Sprintf("token.relay.%d.json", i))
}

// relayLockPath는 i번째 쌍의 중계 모드 advisory lock 경로입니다.
func relayLockPath(i int) string {
	return filepath.Join(configDirFunc(), fmt.Sprintf("token.relay.%d.lock", i))
}

// relayFingerprint는 이번 실행의 i번째 자리표시 쌍 지문입니다.
func relayFingerprint(key string) string {
	h := sha256.Sum256([]byte(relay.RunID() + "\x00" + key))
	return fmt.Sprintf("%x", h)[:16]
}

// GetRelayToken은 PES 중계 모드에서 i번째 쌍의 accessToken을 반환합니다.
// PES에 키가 없거나 i가 범위를 벗어나면 SGIS를 부르지 않고 relay.ErrNoKeys 를 돌려줍니다.
func GetRelayToken(ctx context.Context, i int) (string, error) {
	if !relay.Enabled() {
		return "", errors.New("PES 중계 모드가 아닙니다")
	}
	if i < 0 || i >= relay.KeyCount() {
		return "", relay.ErrNoKeys
	}
	key, secret := relay.Placeholder(i)
	// PES로만 쓰는 사용자는 ~/.sgis 가 없을 수 있어, 락·캐시 파일을 두기 전에 만들어 둡니다.
	_ = config.EnsureConfigDir()
	return getTokenFrom(ctx, credSource{
		key:         key,
		secret:      secret,
		fingerprint: relayFingerprint(key),
		tokenPath:   relayTokenPath(i),
		lockPath:    relayLockPath(i),
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: relay.Wrap(nil),
		},
	})
}

// InvalidateRelayToken은 i번째 쌍의 중계 모드 토큰 캐시를 지워 다음 호출이 재발급하게 합니다.
func InvalidateRelayToken(i int) {
	_ = os.Remove(relayTokenPath(i))
}
