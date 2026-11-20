package cmd

import (
	"fmt"
	"os"

	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/relay"
)

// credentialProblem은 API를 부르기 전에 키가 준비됐는지 확인합니다.
// 문제가 없으면 빈 문자열, 있으면 stderr에 그대로 보여 줄 안내 글을 돌려줍니다.
//
// PES 중계 모드에서는 설정 파일·SGIS_CONSUMER_* 를 보지 않고 PES에 등록된 키 쌍 수만 봅니다.
func credentialProblem() string {
	if relay.Enabled() {
		if relay.KeyCount() == 0 {
			return relay.NoKeyMessage + "\n"
		}
		return ""
	}
	if !config.HasCredentials() {
		return config.NoCredentialMessage()
	}
	return ""
}

// relayErrorText는 err가 중계 쪽 오류(PES에 키 없음 · 중계가 낸 401/403/409)면 보여 줄 글을 돌려줍니다.
// 중계 오류는 SGIS 응답이 아니므로 앞뒤에 다른 말을 붙이지 않고 error 글 그대로 보여 줍니다.
func relayErrorText(err error) (string, bool) {
	if re, ok := relay.AsError(err); ok {
		return re.Message, true
	}
	if relay.IsFatal(err) {
		return relay.NoKeyMessage, true
	}
	return "", false
}

// exitOnRelayError는 err가 중계 쪽 오류면 그 글을 stderr에 찍고 1로 끝냅니다.
func exitOnRelayError(err error) {
	if text, ok := relayErrorText(err); ok {
		fmt.Fprintln(os.Stderr, text)
		os.Exit(1)
	}
}
