package cmd

import (
	"fmt"
	"strings"

	"github.com/clazic/sgis/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "설정 관리 (서비스 ID/보안 Key, 출력 형식 등)",
	Long: `sgis 설정을 관리합니다.

서비스 ID/보안 Key는 SGIS 개발자 포털에서 발급받을 수 있습니다.
https://sgis.kostat.go.kr/developer/html/newOpenApi/api/develop/apiUsageApp.html

하위 명령:
  set-credential KEY SECRET   서비스 ID/보안 Key 설정
  get                         현재 설정 확인 (마스킹)
  set-format FORMAT           기본 출력 형식 설정 (table|json|csv|geojson|xlsx)`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var configSetCredentialCmd = &cobra.Command{
	Use:   "set-credential KEY SECRET",
	Short: "서비스 ID/보안 Key 설정",
	Long: `서비스 ID와 보안 Key를 ~/.sgis/config.yaml에 저장합니다.

설정 파일은 0o600 권한으로 저장되어 소유자만 읽을 수 있습니다.
환경변수 SGIS_CONSUMER_KEY / SGIS_CONSUMER_SECRET를 사용하면 config.yaml보다 우선합니다.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, secret := args[0], args[1]
		if err := config.SetCredentials(key, secret); err != nil {
			return fmt.Errorf("자격증명 저장 실패: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "서비스 ID/보안 Key가 저장되었습니다.")
		fmt.Fprintln(cmd.OutOrStdout(), "다음 명령으로 확인: sgis config get")
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "현재 설정 확인 (자격증명은 마스킹됨)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("설정 로드 실패: %w", err)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "설정 파일: %s\n\n", config.ConfigFilePath())
		fmt.Fprintf(out, "consumer_key:    %s\n", maskValue(cfg.ConsumerKey))
		fmt.Fprintf(out, "consumer_secret: %s\n", maskValue(cfg.ConsumerSecret))
		fmt.Fprintf(out, "default_format:  %s\n", cfg.DefaultFormat)
		fmt.Fprintf(out, "update_check:    %v\n", cfg.UpdateCheck)

		if !config.HasCredentials() {
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, config.NoCredentialMessage())
		}
		return nil
	},
}

var configSetFormatCmd = &cobra.Command{
	Use:   "set-format FORMAT",
	Short: "기본 출력 형식 설정 (table|json|csv|geojson|xlsx)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format := args[0]
		if err := config.SetDefaultFormat(format); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "기본 출력 형식이 '%s'(으)로 설정되었습니다.\n", format)
		return nil
	},
}

// maskValue는 자격증명 값을 마스킹합니다.
// 앞 4자를 보여주고 나머지는 *로 대체합니다.
func maskValue(v string) string {
	if v == "" {
		return "(미설정)"
	}
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return v[:4] + strings.Repeat("*", len(v)-4)
}

func init() {
	configCmd.AddCommand(configSetCredentialCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetFormatCmd)
	rootCmd.AddCommand(configCmd)

	// config 명령은 업데이트 체크를 건너뜁니다.
	// root.go의 skipUpdateCheckCmds에 "config" 키로 이미 등록되어 있습니다.
}
