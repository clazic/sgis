package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/clazic/sgis/internal/api"
	"github.com/clazic/sgis/internal/config"
	"github.com/clazic/sgis/internal/output"
	"github.com/spf13/cobra"
)

func newDataCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "data",
		Short: "통계 데이터 조회 (인구/가구/주택/사업체 등)",
		Long: `SGIS 통계 데이터를 조회합니다.

인구, 가구, 주택, 사업체 통계 및 주제도, 지방통계, 도시권,
생활업종, 기술업종, 성씨, 자연재해 데이터를 포함합니다.

하위 명령 목록:
  sgis data --help 으로 확인하세요.

예시:
  sgis data population --year 2020 --adm-cd 11
  sgis data household --year 2020 --format json
  sgis data company --year 2023 --format csv -o output.csv`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	for _, ep := range api.EndpointsByGroup("data") {
		ep := ep // capture
		sub := buildSubCmd(parent, &ep)
		parent.AddCommand(sub)
	}

	return parent
}

func init() {
	rootCmd.AddCommand(newDataCmd())
}

// buildSubCmd builds a cobra.Command for a single Endpoint.
// It registers string flags for each ep.Params entry and wires RunE
// to collect params, validate required ones, call the API, and render output.
func buildSubCmd(parent *cobra.Command, ep *api.Endpoint) *cobra.Command {
	sub := &cobra.Command{
		Use:   ep.Name,
		Short: ep.Description,
		Long:  buildLongDesc(ep),
		RunE:  makeRunE(ep),
	}

	// Register one string flag per parameter.
	for _, p := range ep.Params {
		flagName := strings.ReplaceAll(p.Name, "_", "-")
		desc := p.Description
		if p.Required {
			desc = "[필수] " + desc
		}
		sub.Flags().String(flagName, "", desc)
	}
	registerParamFlag(sub)

	return sub
}

// registerParamFlag adds the universal --param key=value passthrough flag,
// allowing arbitrary query parameters on any endpoint (including those with
// no predefined Params). Keys are sent verbatim as SGIS query keys.
func registerParamFlag(sub *cobra.Command) {
	sub.Flags().StringArray("param", nil, "임의 쿼리 파라미터 (key=value, 복수 지정 가능)")
}

// mergeExtraParams parses --param entries and merges them into params.
// A defined-flag key already present in params is rejected to avoid ambiguity;
// undefined keys are added as-is. The value may itself contain '=' (split once).
func mergeExtraParams(cmd *cobra.Command, params map[string]string) error {
	extra, err := cmd.Flags().GetStringArray("param")
	if err != nil {
		return nil // flag not registered on this command — nothing to merge
	}
	for _, item := range extra {
		idx := strings.Index(item, "=")
		if idx < 0 {
			return fmt.Errorf("--param 형식 오류: key=value 여야 합니다: %q", item)
		}
		key := strings.TrimSpace(item[:idx])
		val := strings.TrimSpace(item[idx+1:])
		if key == "" {
			return fmt.Errorf("--param 키가 비어있습니다: %q", item)
		}
		if _, exists := params[key]; exists {
			return fmt.Errorf("--param '%s'는 이미 --%s 플래그로 지정됨", key, strings.ReplaceAll(key, "_", "-"))
		}
		params[key] = val
	}
	return nil
}

// buildLongDesc constructs a long description listing all parameters.
func buildLongDesc(ep *api.Endpoint) string {
	var sb strings.Builder
	sb.WriteString(ep.Description)
	sb.WriteString("\n\n경로: ")
	sb.WriteString(ep.Path)
	if len(ep.Params) > 0 {
		sb.WriteString("\n\n파라미터:\n")
		for _, p := range ep.Params {
			req := "선택"
			if p.Required {
				req = "필수"
			}
			flagName := strings.ReplaceAll(p.Name, "_", "-")
			sb.WriteString(fmt.Sprintf("  --%-20s  [%s] %s\n", flagName, req, p.Description))
		}
	}
	return sb.String()
}

// makeRunE returns the RunE handler for an endpoint subcommand.
func makeRunE(ep *api.Endpoint) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Credential check
		if !config.HasCredentials() {
			fmt.Fprint(os.Stderr, config.NoCredentialMessage())
			os.Exit(1)
		}

		// Collect flag values that were set
		params := make(map[string]string)
		var missing []string
		for _, p := range ep.Params {
			flagName := strings.ReplaceAll(p.Name, "_", "-")
			val, err := cmd.Flags().GetString(flagName)
			if err != nil {
				return fmt.Errorf("플래그 읽기 실패 (--%s): %w", flagName, err)
			}
			if val != "" {
				params[p.Name] = val
			} else if p.Required {
				missing = append(missing, "--"+flagName)
			}
		}

		// Merge universal --param passthrough entries
		if err := mergeExtraParams(cmd, params); err != nil {
			return err
		}

		// Validate required params
		if len(missing) > 0 {
			return fmt.Errorf("필수 파라미터가 없습니다: %s", strings.Join(missing, ", "))
		}

		// Determine output format (--format flag > config default > "table")
		format, _ := cmd.Root().PersistentFlags().GetString("format")
		if format == "" || format == "table" {
			if cfg, err := config.Load(); err == nil && cfg.DefaultFormat != "" {
				format = cfg.DefaultFormat
			}
		}

		// Retrieve -o / --output flag (registered on root or this command)
		outFile, _ := cmd.Flags().GetString("output")
		if outFile == "" {
			outFile, _ = cmd.Root().PersistentFlags().GetString("output")
		}

		// Call API
		client, err := api.NewClient()
		if err != nil {
			return fmt.Errorf("API 클라이언트 초기화 실패: %w", err)
		}
		result, err := client.Call(context.Background(), ep, params)
		if err != nil {
			return fmt.Errorf("API 호출 실패: %w", err)
		}

		// Render
		return output.Render(result, output.Options{
			Format:  format,
			OutFile: outFile,
			Title:   ep.Name,
		})
	}
}
