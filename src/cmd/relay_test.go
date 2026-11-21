package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/relay"
)

func TestCredentialProblem(t *testing.T) {
	config.SetConfigDirForTesting(t.TempDir())
	t.Cleanup(func() { config.SetConfigDirForTesting("") })
	t.Setenv("SGIS_CONSUMER_KEY", "")
	t.Setenv("SGIS_CONSUMER_SECRET", "")

	// 중계 모드 아님 + 키 없음 → 기존 안내
	t.Setenv("PES_RELAY_URL", "")
	if got := credentialProblem(); got != config.NoCredentialMessage() {
		t.Errorf("기존 안내가 바뀜: %q", got)
	}
	// 중계 모드 아님 + 키 있음 → 문제 없음
	t.Setenv("SGIS_CONSUMER_KEY", "k")
	t.Setenv("SGIS_CONSUMER_SECRET", "s")
	if got := credentialProblem(); got != "" {
		t.Errorf("키가 있는데 문제로 판단: %q", got)
	}

	// 중계 모드 + PES 키 0개 → 자기 키가 있어도 규약의 안내
	t.Setenv("PES_RELAY_URL", "http://127.0.0.1:1")
	for _, n := range []string{"", "0"} {
		t.Setenv("PES_RELAY_SGIS_KEYS", n)
		want := "PES 에 SGIS 키가 없어요. PES 「도구」에서 키를 넣으세요.\n"
		if got := credentialProblem(); got != want {
			t.Errorf("[%q] 안내 글 불일치: %q", n, got)
		}
	}
	// 중계 모드 + PES 키 있음 → 자기 키가 없어도 문제 없음
	t.Setenv("SGIS_CONSUMER_KEY", "")
	t.Setenv("SGIS_CONSUMER_SECRET", "")
	t.Setenv("PES_RELAY_SGIS_KEYS", "2")
	if got := credentialProblem(); got != "" {
		t.Errorf("PES 키가 있는데 문제로 판단: %q", got)
	}
}

func TestRelayErrorText(t *testing.T) {
	wrapped := fmt.Errorf("API 호출 실패: %w", &relay.Error{Status: 401, Message: "토큰이 맞지 않아요"})
	if text, ok := relayErrorText(wrapped); !ok || text != "토큰이 맞지 않아요" {
		t.Errorf("중계 오류 글 불일치: %q %v", text, ok)
	}
	if text, ok := relayErrorText(fmt.Errorf("감쌈: %w", relay.ErrNoKeys)); !ok || text != relay.NoKeyMessage {
		t.Errorf("키 없음 글 불일치: %q %v", text, ok)
	}
	if _, ok := relayErrorText(errors.New("SGIS API 오류 (errCd=-100)")); ok {
		t.Error("보통 오류를 중계 오류로 판단")
	}
}

func TestShouldCheckUpdate_OffInRelayMode(t *testing.T) {
	t.Setenv("SGIS_NO_UPDATE_CHECK", "")
	t.Setenv("PES_RELAY_URL", "http://127.0.0.1:1")
	if shouldCheckUpdate() {
		t.Error("중계 모드인데 업데이트 확인이 켜짐")
	}
}
