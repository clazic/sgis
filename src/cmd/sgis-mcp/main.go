// Command sgis-mcp는 SGIS Open API를 읽기 전용 MCP 도구로 내놓는 stdio MCP 서버입니다.
//
// 기본 키: SGIS_API_KEYS (쉼표 구분, 각 항목 consumer_key:consumer_secret).
// 없으면 sgis CLI 설정(SGIS_CONSUMER_KEY/SECRET 환경변수 → ~/.sgis/config.yaml)을 씁니다.
// 요청별 키: tools/call params._meta["kr.pes/keys"] (같은 형식의 문자열 배열).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/mcp"
)

var version = "dev"

const usage = `sgis-mcp — SGIS 통계지리정보서비스 stdio MCP 서버 (읽기 전용)

사용법:
  sgis-mcp            stdin/stdout 으로 MCP(JSON-RPC 2.0, 줄 단위) 서버 실행
  sgis-mcp --version  버전 출력
  sgis-mcp --help     이 도움말

환경변수:
  SGIS_API_KEYS  consumer_key:consumer_secret 를 쉼표로 여러 개 (키 풀, 라운드로빈)
                 없으면 sgis CLI 설정(SGIS_CONSUMER_KEY/SECRET, ~/.sgis/config.yaml)을 씀
요청별 키:
  tools/call 의 params._meta["kr.pes/keys"] = ["key:secret", ...] (있으면 그 키만 씀)
`

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "--version", "-v", "version":
			fmt.Println(version)
			return
		case "--help", "-h", "help":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "알 수 없는 인자입니다. --help 를 보세요.\n")
			os.Exit(2)
		}
	}

	logger := log.New(os.Stderr, "sgis-mcp: ", log.LstdFlags)
	log.SetOutput(os.Stderr) // 내부 패키지의 log 출력도 stdout 을 더럽히지 않게

	keys := loadDefaultKeys(logger)
	srv := mcp.NewServer(mcp.Options{Version: version, DefaultKeys: keys, Logger: logger})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		logger.Printf("입력 읽기 오류로 종료합니다")
		os.Exit(1)
	}
}

// loadDefaultKeys는 기본 키 풀을 만듭니다. 키 원문은 어디에도 찍지 않습니다.
func loadDefaultKeys(logger *log.Logger) []mcp.Cred {
	if v := os.Getenv("SGIS_API_KEYS"); v != "" {
		creds, err := mcp.ParseEnvKeys(v)
		if err != nil {
			logger.Printf("SGIS_API_KEYS 형식 오류(%v) — 기본 키 없이 시작합니다", err)
			return nil
		}
		logger.Printf("기본 키 %d개(SGIS_API_KEYS)", len(creds))
		return creds
	}
	cfg, err := config.Load()
	if err == nil && cfg.ConsumerKey != "" && cfg.ConsumerSecret != "" {
		logger.Printf("기본 키 1개(sgis CLI 설정)")
		return []mcp.Cred{{Key: cfg.ConsumerKey, Secret: cfg.ConsumerSecret}}
	}
	logger.Printf("기본 키 없음 — 요청별 키(_meta[\"kr.pes/keys\"])만 쓸 수 있어요")
	return nil
}
