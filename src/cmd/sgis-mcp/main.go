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
	"strings"
	"syscall"

	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/mcp"
)

var version = "dev"

const usage = `sgis-mcp — SGIS 통계지리정보서비스 stdio MCP 서버 (읽기 전용)

사용법:
  sgis-mcp            stdin/stdout 으로 MCP(JSON-RPC 2.0, 줄 단위) 서버 실행
  sgis-mcp --http [주소]
                      Streamable HTTP 단독 서버 (경로 /mcp, 기본 127.0.0.1:8080)
  sgis-mcp --version  버전 출력
  sgis-mcp --help     이 도움말

환경변수:
  SGIS_API_KEYS  consumer_key:consumer_secret 를 쉼표로 여러 개 (키 풀, 라운드로빈)
                 없으면 sgis CLI 설정(SGIS_CONSUMER_KEY/SECRET, ~/.sgis/config.yaml)을 씀
  SGIS_MCP_TOKEN            HTTP 모드: 있으면 Authorization: Bearer <토큰> 필수.
                            loopback 이 아닌 주소는 이 값 없이는 시작 거부
  SGIS_MCP_ALLOWED_ORIGINS  HTTP 모드: loopback 외에 허용할 Origin (쉼표)
  MCP_RESULTS_DIR           큰 결과 저장 폴더 (기본 ~/.sgis/cache/mcp-results)
  MCP_RESULTS_TTL           결과 보관 시간 (기본 1h; 3600 같은 초도 됨)
  MCP_RESULTS_MAX_BYTES     결과 폴더 상한 바이트 (기본 1073741824 = 1GB)
요청별 키:
  tools/call 의 params._meta["kr.pes/keys"] = ["key:secret", ...] (있으면 그 키만 씀)
  HTTP 모드에서는 머리글 X-Api-Keys: key:secret,key2:secret2 도 같음
`

func main() {
	httpMode, addr := false, "127.0.0.1:8080"
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--version" || a == "-v" || a == "version":
			fmt.Println(version)
			return
		case a == "--help" || a == "-h" || a == "help":
			fmt.Print(usage)
			return
		case a == "--http":
			httpMode = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				addr = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--http="):
			httpMode, addr = true, strings.TrimPrefix(a, "--http=")
		default:
			fmt.Fprintf(os.Stderr, "알 수 없는 인자입니다. --help 를 보세요.\n")
			os.Exit(2)
		}
	}

	logger := log.New(os.Stderr, "sgis-mcp: ", log.LstdFlags)
	log.SetOutput(os.Stderr) // 내부 패키지의 log 출력도 stdout 을 더럽히지 않게

	keys := loadDefaultKeys(logger)
	results, err := mcp.ResultStoreFromEnv()
	if err != nil {
		logger.Printf("시작 거부: %v", err)
		os.Exit(1)
	}
	srv := mcp.NewServer(mcp.Options{Version: version, DefaultKeys: keys, Logger: logger, Results: results})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go results.RunCleanup(ctx) // 시작할 때와 10분마다 만료·용량 정리

	if httpMode {
		o := mcp.HTTPOptions{
			Token:          os.Getenv("SGIS_MCP_TOKEN"),
			AllowedOrigins: mcp.ParseOrigins(os.Getenv("SGIS_MCP_ALLOWED_ORIGINS")),
			AccessLog:      logger,
		}
		if err := mcp.CheckBind(addr, o.Token); err != nil {
			logger.Printf("시작 거부: %v", err)
			os.Exit(1)
		}
		logger.Printf("HTTP 모드: http://%s/mcp (토큰 %s)", addr, map[bool]string{true: "필수", false: "없음"}[o.Token != ""])
		if err := srv.RunHTTP(ctx, addr, o); err != nil {
			logger.Printf("HTTP 서버 오류: %v", err)
			os.Exit(1)
		}
		return
	}

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
