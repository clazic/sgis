package cmd

import (
	"strings"

	"github.com/clazic/sgis/internal/api"
	"github.com/spf13/cobra"
)

func newCodeCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "code",
		Short: "행정구역 코드 조회 (시도/시군구/읍면동)",
		Long: `SGIS 행정구역 코드 및 기준년도를 조회합니다.

하위 명령:
  stage                단계별 주소 조회 (시도/시군구/읍면동 코드)
  findcodeinsmallarea  소지역 코드찾기
  year-data            최신/전체 기준년도 조회
  industrycode         산업분류 코드

예시:
  sgis code stage                          # 시도 목록 조회
  sgis code stage --cd 11                  # 서울 시군구 목록
  sgis code stage --cd 11010              # 종로구 읍면동 목록
  sgis code year-data                      # 가용 기준년도 목록
  sgis code industrycode --class-deg 10    # 산업분류 코드 조회`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	for _, ep := range api.EndpointsByGroup("code") {
		ep := ep // capture
		sub := &cobra.Command{
			Use:   ep.Name,
			Short: ep.Description,
			Long:  buildLongDesc(&ep),
			RunE:  makeRunE(&ep),
		}
		for _, p := range ep.Params {
			flagName := strings.ReplaceAll(p.Name, "_", "-")
			desc := p.Description
			if p.Required {
				desc = "[필수] " + desc
			}
			sub.Flags().String(flagName, "", desc)
		}
		parent.AddCommand(sub)
	}

	return parent
}

func init() {
	rootCmd.AddCommand(newCodeCmd())
}
