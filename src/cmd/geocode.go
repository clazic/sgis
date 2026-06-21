package cmd

import (
	"github.com/clazic/sgis/internal/api"
	"github.com/spf13/cobra"
	"strings"
)

func newGeocodeCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "geocode",
		Short: "주소→좌표 변환 및 좌표계 변환",
		Long: `SGIS 지오코딩 및 좌표변환 API를 호출합니다.

하위 명령:
  geocode       주소 → UTM-K 좌표 (EPSG:5179)
  geocodewgs84  주소 → WGS84 좌표 (EPSG:4326)
  rgeocode      UTM-K 좌표 → 주소
  rgeocodewgs84 WGS84 좌표 → 주소
  transcoord    좌표계 변환

예시:
  sgis geocode geocode --address "서울특별시 종로구"
  sgis geocode geocodewgs84 --address "부산광역시 해운대구"
  sgis geocode rgeocode --x-coor 953932 --y-coor 1952053
  sgis geocode transcoord --src 5179 --dst 4326 --posX 953932 --posY 1952053`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	for _, ep := range api.EndpointsByGroup("geocode") {
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
	rootCmd.AddCommand(newGeocodeCmd())
}
