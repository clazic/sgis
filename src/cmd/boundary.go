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

func newBoundaryCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "boundary",
		Short: "행정구역 경계 조회 (GeoJSON 출력 지원)",
		Long: `SGIS 행정구역 경계 데이터를 조회합니다.

좌표계: UTM-K (EPSG:5179). Leaflet 등 웹지도에서 바로 쓰려면 --wgs84 플래그로
WGS84(EPSG:4326)로 재투영하세요. 기본 출력 형식은 geojson입니다.

하위 명령:
  hadmarea             행정구역경계
  statsarea            집계구경계
  userarea             영역내경계
  urban-boundary       도시/준도시 경계
  grid-data            행정구역 격자경계
  figure-buildingarea  전개도 건물경계
  figure-floorboundary 층별 최외각 공간속성
  figure-floorcompany  층별 사업체 공간속성

예시:
  sgis boundary hadmarea --year 2024 --adm-cd 11
  sgis boundary hadmarea --year 2024 --adm-cd 11 --format geojson -o seoul.geojson
  sgis boundary hadmarea --year 2024 --adm-cd 11 --wgs84 -o seoul.geojson
  sgis boundary statsarea --adm-cd 11010530`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	for _, ep := range api.EndpointsByGroup("boundary") {
		ep := ep // capture
		sub := &cobra.Command{
			Use:   ep.Name,
			Short: ep.Description,
			Long:  buildLongDesc(&ep),
			RunE:  makeBoundaryRunE(&ep),
		}
		for _, p := range ep.Params {
			flagName := strings.ReplaceAll(p.Name, "_", "-")
			desc := p.Description
			if p.Required {
				desc = "[필수] " + desc
			}
			sub.Flags().String(flagName, "", desc)
		}
		registerParamFlag(sub)
		sub.Flags().Bool("wgs84", false, "좌표를 WGS84(EPSG:4326)로 재투영 (Leaflet 등 웹지도용, geojson 전용)")
		parent.AddCommand(sub)
	}

	return parent
}

// makeBoundaryRunE is like makeRunE but defaults to "geojson" format instead of "table".
func makeBoundaryRunE(ep *api.Endpoint) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if !config.HasCredentials() {
			fmt.Fprint(os.Stderr, config.NoCredentialMessage())
			os.Exit(1)
		}

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
		if err := mergeExtraParams(cmd, params); err != nil {
			return err
		}
		if len(missing) > 0 {
			return fmt.Errorf("필수 파라미터가 없습니다: %s", strings.Join(missing, ", "))
		}

		// Format resolution: explicit --format > config default > "geojson" (boundary default)
		format, _ := cmd.Root().PersistentFlags().GetString("format")
		if format == "" || format == "table" {
			if cfg, err := config.Load(); err == nil && cfg.DefaultFormat != "" && cfg.DefaultFormat != "table" {
				format = cfg.DefaultFormat
			} else {
				format = "geojson"
			}
		}

		outFile, _ := cmd.Flags().GetString("output")
		if outFile == "" {
			outFile, _ = cmd.Root().PersistentFlags().GetString("output")
		}

		client, err := api.NewClient()
		if err != nil {
			return fmt.Errorf("API 클라이언트 초기화 실패: %w", err)
		}
		result, err := client.Call(context.Background(), ep, params)
		if err != nil {
			return fmt.Errorf("API 호출 실패: %w", err)
		}

		wgs84, _ := cmd.Flags().GetBool("wgs84")

		return output.Render(result, output.Options{
			Format:  format,
			OutFile: outFile,
			Title:   ep.Name,
			WGS84:   wgs84,
		})
	}
}

func init() {
	rootCmd.AddCommand(newBoundaryCmd())
}
