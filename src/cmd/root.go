package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var appVersion = "dev"

const rootHelpText = `SGIS CLI - 통계지리정보서비스 Open API 조회 도구

SGIS(통계지리정보서비스) Open API 기반 CLI 도구입니다.
통계(인구/가구/주택/사업체), 행정경계(GeoJSON), 지오코딩/좌표변환을 지원합니다.

사용법:
  sgis [command]

명령 그룹:
  data      통계 데이터 조회 (인구/가구/주택/사업체)
  boundary  행정구역 경계 조회 (GeoJSON 출력 지원)
  geocode   주소→좌표 변환 및 좌표계 변환
  code      행정구역 코드 조회 (시도/시군구)
  config    설정 관리 (서비스 ID/보안 Key)
  update    CLI 업데이트

플래그:
  -v, --version   버전 정보
  -h, --help      도움말

시작하기:
  # 1. 서비스 ID/보안 Key 설정 (https://sgis.kostat.go.kr/developer/ 에서 발급)
  sgis config set-credential <서비스 ID> <보안 Key>

  # 2. 인구 통계 조회
  sgis data population --adm-cd 11 --year 2020

  # 3. 행정경계 GeoJSON 조회
  sgis boundary hadmarea --format geojson

  # 4. 주소 지오코딩
  sgis geocode address "서울특별시 종로구"

더 알아보기:
  sgis <command> --help   각 명령 상세 도움말
`

// SetVersion sets the application version (called from main with ldflags-injected value).
func SetVersion(v string) {
	appVersion = v
}

// skipUpdateCheckCmds lists commands that skip the background update check.
var skipUpdateCheckCmds = map[string]bool{
	"update":  true,
	"version": true,
	"config":  true,
}

var rootCmd = &cobra.Command{
	Use:     "sgis",
	Short:   "SGIS CLI - 통계지리정보서비스 Open API 조회 도구",
	Long:    rootHelpText,
	Version: appVersion,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		name := cmd.Name()
		if skipUpdateCheckCmds[name] {
			return
		}
		startBackgroundUpdateCheck()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		name := cmd.Name()
		if skipUpdateCheckCmds[name] {
			return
		}
		printUpdateNotice()
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), rootHelpText)
	},
}

// Execute runs the root command.
func Execute() {
	rootCmd.Version = appVersion
	rootCmd.SetArgs(os.Args[1:])
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Persistent flags available to all subcommands.
	rootCmd.PersistentFlags().StringP("format", "f", "table", "출력 포맷: table|json|csv|geojson|xlsx")
	rootCmd.PersistentFlags().StringP("output", "o", "", "출력 파일 경로 (미지정 시 stdout)")
}
