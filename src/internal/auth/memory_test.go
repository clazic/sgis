package auth

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestMemoryToken_CachesPerKeyWithoutFile는 MemoryToken이 키별로 따로 발급·캐시하고
// token.json 파일을 만들지 않는지 검증합니다.
func TestMemoryToken_CachesPerKeyWithoutFile(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	var calls atomic.Int32
	srv := makeAuthServer(t, &calls, 0, "")
	defer srv.Close()

	a := NewMemoryToken("key-a", "secret-a")
	a.AuthURL = srv.URL
	b := NewMemoryToken("key-b", "secret-b")
	b.AuthURL = srv.URL

	ta1, err := a.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ta2, _ := a.Token(context.Background())
	tb, _ := b.Token(context.Background())
	if ta1 != ta2 {
		t.Errorf("같은 키는 캐시 재사용: %s vs %s", ta1, ta2)
	}
	if ta1 == tb {
		t.Error("다른 키는 다른 토큰이어야 함")
	}
	if calls.Load() != 2 {
		t.Errorf("발급 2회 기대, 실제 %d", calls.Load())
	}
	a.Invalidate()
	if ta3, _ := a.Token(context.Background()); ta3 == ta1 {
		t.Error("Invalidate 후 재발급해야 함")
	}
	if _, err := os.Stat(filepath.Join(dir, "token.json")); !os.IsNotExist(err) {
		t.Error("MemoryToken은 token.json을 만들면 안 됨")
	}
}

func TestMemoryToken_InvalidCredential(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()
	var calls atomic.Int32
	srv := makeAuthServer(t, &calls, -401, "bad")
	defer srv.Close()
	m := NewMemoryToken("k", "s")
	m.AuthURL = srv.URL
	if _, err := m.Token(context.Background()); !isErrInvalidCredential(err) {
		t.Errorf("ErrInvalidCredential 기대: %v", err)
	}
}
