package cmd

import (
	"github.com/clazic/sgis/internal/api"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "search",
		Short: "검색 (연관어/SOP)",
		Long: `SGIS 검색 API를 호출합니다.

하위 명령:
  relword  연관어검색 (검색어→유의어)
  sop      SOP검색 (검색어→통계 SOP 정보)

예시:
  sgis search relword --searchword 주택
  sgis search sop --searchword 주택 --resultcount 10`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	for _, ep := range api.EndpointsByGroup("search") {
		ep := ep // capture
		sub := buildSubCmd(parent, &ep)
		parent.AddCommand(sub)
	}

	return parent
}

func init() {
	rootCmd.AddCommand(newSearchCmd())
}
