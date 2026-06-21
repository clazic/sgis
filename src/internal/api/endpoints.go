// Package api는 SGIS Open API 엔드포인트 레지스트리와 HTTP 클라이언트를 제공합니다.
// 엔드포인트 정보는 references/sgis-endpoints.md 를 검토한 정적 정의입니다.
package api

// Param은 엔드포인트 파라미터 하나를 나타냅니다.
type Param struct {
	Name        string
	Required    bool
	Description string
}

// Endpoint는 SGIS Open API 엔드포인트 하나를 나타냅니다.
// Path는 OpenAPI3 베이스(https://sgisapi.mods.go.kr/OpenAPI3/) 기준 상대 경로입니다.
// accessToken 파라미터는 Client가 자동으로 주입하므로 Params에 포함하지 않습니다.
type Endpoint struct {
	Group       string  // "data" | "boundary" | "geocode" | "code" | "auth"
	Name        string  // 명령 식별자 (slug)
	Path        string  // OpenAPI3 베이스 기준 상대 경로
	Method      string  // "GET" (현재 모든 SGIS 엔드포인트)
	Description string  // 한국어 설명
	Params      []Param // accessToken 제외한 파라미터 목록
}

// Registry는 SGIS Open API 엔드포인트 전체 정적 레지스트리입니다.
// 출처: references/sgis-endpoints.md (2026-06-21 스크랩·검토).
var Registry = []Endpoint{

	// ── auth ──────────────────────────────────────────────────────────────────
	// 인증은 internal/auth 패키지가 담당하지만, 완전성을 위해 등록합니다.
	{
		Group: "auth", Name: "authentication", Method: "GET",
		Path:        "auth/authentication.json",
		Description: "accessToken 발급",
		Params: []Param{
			{Name: "consumer_key", Required: true, Description: "서비스 ID"},
			{Name: "consumer_secret", Required: true, Description: "서비스 Secret"},
		},
	},
	{
		Group: "auth", Name: "javascript-auth", Method: "GET",
		Path:        "auth/javascriptAuth.json",
		Description: "JavaScript API 인증",
		Params: []Param{
			{Name: "consumer_key", Required: true, Description: "서비스 ID"},
		},
	},

	// ── data — 통계 (stats) ───────────────────────────────────────────────────
	{
		Group: "data", Name: "population", Method: "GET",
		Path:        "stats/population.json",
		Description: "인구통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도 (2015~2024)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드 (미지정=전국)"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함 (0=해당, 1=1단계, 2=2단계)"},
		},
	},
	{
		Group: "data", Name: "searchpopulation", Method: "GET",
		Path:        "stats/searchpopulation.json",
		Description: "인구검색(검색조건)",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "gender", Required: false, Description: "성별"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "age_type", Required: false, Description: "나이 유형"},
			{Name: "edu_level", Required: false, Description: "교육수준"},
			{Name: "mrg_state", Required: false, Description: "혼인상태"},
		},
	},
	{
		Group: "data", Name: "household", Method: "GET",
		Path:        "stats/household.json",
		Description: "가구통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "household_type", Required: false, Description: "가구 유형"},
			{Name: "ocptn_type", Required: false, Description: "직업 유형"},
		},
	},
	{
		Group: "data", Name: "house", Method: "GET",
		Path:        "stats/house.json",
		Description: "주택통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "house_type", Required: false, Description: "주택 유형"},
			{Name: "const_year", Required: false, Description: "건축연도"},
			{Name: "house_area_cd", Required: false, Description: "주택면적코드"},
		},
	},
	{
		Group: "data", Name: "company", Method: "GET",
		Path:        "stats/company.json",
		Description: "사업체통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도 (2000~2024)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "class_code", Required: false, Description: "산업분류코드"},
			{Name: "theme_cd", Required: false, Description: "테마코드"},
		},
	},
	{
		Group: "data", Name: "industrycode", Method: "GET",
		Path:        "stats/industrycode.json",
		Description: "산업분류코드",
		Params: []Param{
			{Name: "class_deg", Required: true, Description: "산업분류 차수"},
			{Name: "class_code", Required: false, Description: "상위 분류코드"},
		},
	},
	{
		Group: "data", Name: "farmhousehold", Method: "GET",
		Path:        "stats/farmhousehold.json",
		Description: "농가통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도 (2000/2005/2010/2015/2020)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
		},
	},
	{
		Group: "data", Name: "forestryhousehold", Method: "GET",
		Path:        "stats/forestryhousehold.json",
		Description: "임가통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
		},
	},
	{
		Group: "data", Name: "fisheryhousehold", Method: "GET",
		Path:        "stats/fisheryhousehold.json",
		Description: "어가통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "oga_div", Required: true, Description: "어업 구분"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
		},
	},
	{
		Group: "data", Name: "householdmember", Method: "GET",
		Path:        "stats/householdmember.json",
		Description: "가구원통계",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "data_type", Required: true, Description: "데이터 유형"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "gender", Required: false, Description: "성별"},
			{Name: "age_from", Required: false, Description: "나이 시작"},
			{Name: "age_to", Required: false, Description: "나이 끝"},
		},
	},

	// ── data — 주제도 (themamap) ──────────────────────────────────────────────
	{
		Group: "data", Name: "themamap-ctgr001-list", Method: "GET",
		Path:        "themamap/CTGR_001/list.json",
		Description: "인구와 가구 주제도 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr001-data", Method: "GET",
		Path:        "themamap/CTGR_001/data.json",
		Description: "인구와 가구 주제도 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr002-list", Method: "GET",
		Path:        "themamap/CTGR_002/list.json",
		Description: "주거와 교통 주제도 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr002-data", Method: "GET",
		Path:        "themamap/CTGR_002/data.json",
		Description: "주거와 교통 주제도 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr003-list", Method: "GET",
		Path:        "themamap/CTGR_003/list.json",
		Description: "복지와 문화 주제도 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr003-data", Method: "GET",
		Path:        "themamap/CTGR_003/data.json",
		Description: "복지와 문화 주제도 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr004-list", Method: "GET",
		Path:        "themamap/CTGR_004/list.json",
		Description: "노동과 경제 주제도 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "themamap-ctgr004-data", Method: "GET",
		Path:        "themamap/CTGR_004/data.json",
		Description: "노동과 경제 주제도 상세",
		Params:      []Param{},
	},

	// ── data — 지방통계 (jibang) ──────────────────────────────────────────────
	{
		Group: "data", Name: "jibang-a-list", Method: "GET",
		Path:        "jibang/category_a/list.json",
		Description: "가구/인구비율 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-a-data", Method: "GET",
		Path:        "jibang/category_a/data.json",
		Description: "가구/인구비율 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-b-list", Method: "GET",
		Path:        "jibang/category_b/list.json",
		Description: "사회비율 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-b-data", Method: "GET",
		Path:        "jibang/category_b/data.json",
		Description: "사회비율 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-c-list", Method: "GET",
		Path:        "jibang/category_c/list.json",
		Description: "주택비율 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-c-data", Method: "GET",
		Path:        "jibang/category_c/data.json",
		Description: "주택비율 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-d-list", Method: "GET",
		Path:        "jibang/category_d/list.json",
		Description: "교통비율 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-d-data", Method: "GET",
		Path:        "jibang/category_d/data.json",
		Description: "교통비율 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-e-list", Method: "GET",
		Path:        "jibang/category_e/list.json",
		Description: "종교비율 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-e-data", Method: "GET",
		Path:        "jibang/category_e/data.json",
		Description: "종교비율 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-f-list", Method: "GET",
		Path:        "jibang/category_f/list.json",
		Description: "사업체비율 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "jibang-f-data", Method: "GET",
		Path:        "jibang/category_f/data.json",
		Description: "사업체비율 상세",
		Params:      []Param{},
	},

	// ── data — 도시권 (urban) ─────────────────────────────────────────────────
	{
		Group: "data", Name: "urban-category", Method: "GET",
		Path:        "urban/category.json",
		Description: "도시권 목록(카테고리)",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-list", Method: "GET",
		Path:        "urban/list.json",
		Description: "도시/준도시 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-ingu", Method: "GET",
		Path:        "urban/ingu/data.json",
		Description: "도시별 인구 통계",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-gagu", Method: "GET",
		Path:        "urban/gagu/data.json",
		Description: "도시별 가구 통계",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-ho", Method: "GET",
		Path:        "urban/ho/data.json",
		Description: "도시별 주택 통계",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-corp", Method: "GET",
		Path:        "urban/corp/data.json",
		Description: "사업체 통계",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-to", Method: "GET",
		Path:        "urban/to/data.json",
		Description: "주요지표 통계",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "urban-fac", Method: "GET",
		Path:        "urban/fac/data.json",
		Description: "도시별 생활시설 통계",
		Params:      []Param{},
	},

	// ── data — 생활업종 (startupbiz) ──────────────────────────────────────────
	{
		Group: "data", Name: "startupbiz", Method: "GET",
		Path:        "startupbiz/startupbiz.json",
		Description: "생활업종 후보지검색",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-pplsummary", Method: "GET",
		Path:        "startupbiz/pplsummary.json",
		Description: "거주인구 요약",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-mfratiosummary", Method: "GET",
		Path:        "startupbiz/mfratiosummary.json",
		Description: "성별인구비율 요약",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-housesummary", Method: "GET",
		Path:        "startupbiz/housesummary.json",
		Description: "거처종류 요약",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-corpdistsummary", Method: "GET",
		Path:        "startupbiz/corpdistsummary.json",
		Description: "소상공인 업종별 사업체비율",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-corpindecrease", Method: "GET",
		Path:        "startupbiz/corpindecrease.json",
		Description: "소상공인 업종별 사업체증감",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-regiontotal", Method: "GET",
		Path:        "startupbiz/regiontotal.json",
		Description: "생활업종 후보지 정보",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-sidotobrank", Method: "GET",
		Path:        "startupbiz/sidotobrank.json",
		Description: "시도별 생활업종 순위",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-sidotobinfo", Method: "GET",
		Path:        "startupbiz/sidotobinfo.json",
		Description: "시도별 생활업종 정보",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-sidotobgroup", Method: "GET",
		Path:        "startupbiz/sidotobgroup.json",
		Description: "시도별 생활업종 속성",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-sggtobcorpcount", Method: "GET",
		Path:        "startupbiz/sggtobcorpcount.json",
		Description: "시군구별 생활업종 사업체수",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-sggtobinfo", Method: "GET",
		Path:        "startupbiz/sggtobinfo.json",
		Description: "시군구별 생활업종 정보",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-sggtobrank", Method: "GET",
		Path:        "startupbiz/sggtobrank.json",
		Description: "생활업종별 시군구 순위",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "startupbiz-compareregiontotal", Method: "GET",
		Path:        "startupbiz/Compareregiontotal.json",
		Description: "생활업종 후보지 비교",
		Params:      []Param{},
	},

	// ── data — 기술업종 (technicalbiz) ────────────────────────────────────────
	{
		Group: "data", Name: "technicalbiz-companyinfo", Method: "GET",
		Path:        "technicalbiz/companyinfo.json",
		Description: "전국 기술업종 정보",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "technicalbiz-sidocompanyinfo", Method: "GET",
		Path:        "technicalbiz/sidocompanyinfo.json",
		Description: "시도별 기술업종 정보",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "technicalbiz-sggcompanyinfo", Method: "GET",
		Path:        "technicalbiz/sggcompanyinfo.json",
		Description: "시군구별 기술업종 정보",
		Params:      []Param{},
	},

	// ── data — 성씨 (lastname) ────────────────────────────────────────────────
	{
		Group: "data", Name: "lastname-list", Method: "GET",
		Path:        "lastname/list.json",
		Description: "50대 성씨 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "lastname-data", Method: "GET",
		Path:        "lastname/data.json",
		Description: "50대 성씨 시군구별 인구수",
		Params:      []Param{},
	},

	// ── data — 지역현안 소통지도 (statscommunity) ─────────────────────────────
	{
		Group: "data", Name: "statscommunity-list", Method: "GET",
		Path:        "statscommunity/list.json",
		Description: "지역현안 소통지도 목록",
		Params:      []Param{},
	},

	// ── data — 자연재해 (ndsm) ────────────────────────────────────────────────
	{
		Group: "data", Name: "ndsm-typhoon-year-list", Method: "GET",
		Path:        "ndsm/typInfoYearList.json",
		Description: "년도별 태풍정보 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-typhoon-adm-list", Method: "GET",
		Path:        "ndsm/typInfoAdmCdList.json",
		Description: "태풍별 영향범위 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-typhoon-data", Method: "GET",
		Path:        "ndsm/typDataBoard.json",
		Description: "태풍별 통계정보 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-flood-adm-list", Method: "GET",
		Path:        "ndsm/floodRiskAdmCdList.json",
		Description: "홍수위험지도 영향범위 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-flood-data", Method: "GET",
		Path:        "ndsm/floodRiskDataBoard.json",
		Description: "홍수위험지도 통계정보 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-landslide-adm-list", Method: "GET",
		Path:        "ndsm/lndsldWarnAdmCdList.json",
		Description: "산사태위험지도 영향범위 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-landslide-data", Method: "GET",
		Path:        "ndsm/lndsldWarnDataBoard.json",
		Description: "산사태위험지도 통계정보 상세",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-heatwave-spcnws-list", Method: "GET",
		Path:        "ndsm/prevHwSpcnwsList.json",
		Description: "과거 폭염특보 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-heatwave-impact-list", Method: "GET",
		Path:        "ndsm/prevHwImpctFrcstList.json",
		Description: "과거 폭염 영향예보 목록",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "ndsm-heatwave-data", Method: "GET",
		Path:        "ndsm/hwDataBoard.json",
		Description: "폭염 통계정보 상세",
		Params:      []Param{},
	},

	// ── data — 전개도 건물정보 (figure, json) ─────────────────────────────────
	{
		Group: "data", Name: "figure-buildingattribute", Method: "GET",
		Path:        "figure/buildingattribute.json",
		Description: "건물 정보",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "figure-flooretcfacility", Method: "GET",
		Path:        "figure/flooretcfacility2.json",
		Description: "층별 시설물 공간속성",
		Params:      []Param{},
	},
	{
		Group: "data", Name: "figure-floorcompanyinfo", Method: "GET",
		Path:        "figure/floorcompanyinfo.json",
		Description: "건물층별 사업체정보",
		Params:      []Param{},
	},

	// ── boundary — 경계 (GeoJSON) ─────────────────────────────────────────────
	// 좌표계: UTM-K (EPSG:5179). Leaflet 지도 사용 시 WGS84 재투영 필요.
	{
		Group: "boundary", Name: "hadmarea", Method: "GET",
		Path:        "boundary/hadmarea.geojson",
		Description: "행정구역경계 (UTM-K GeoJSON)",
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도 (2000~2025)"},
			{Name: "adm_cd", Required: true, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
		},
	},
	{
		Group: "boundary", Name: "statsarea", Method: "GET",
		Path:        "boundary/statsarea.geojson",
		Description: "집계구경계 (UTM-K GeoJSON)",
		Params: []Param{
			{Name: "adm_cd", Required: true, Description: "행정구역코드 (8자리)"},
		},
	},
	{
		Group: "boundary", Name: "userarea", Method: "GET",
		Path:        "boundary/userarea.geojson",
		Description: "영역내경계 (UTM-K GeoJSON)",
		Params: []Param{
			{Name: "minx", Required: true, Description: "최소 X 좌표 (UTM-K)"},
			{Name: "miny", Required: true, Description: "최소 Y 좌표 (UTM-K)"},
			{Name: "maxx", Required: true, Description: "최대 X 좌표 (UTM-K)"},
			{Name: "maxy", Required: true, Description: "최대 Y 좌표 (UTM-K)"},
			{Name: "cd", Required: true, Description: "단위 (1=시도/2=시군구/3=읍면동/4=집계구)"},
		},
	},
	{
		Group: "boundary", Name: "urban-boundary", Method: "GET",
		Path:        "urban/boundary.geojson",
		Description: "도시/준도시 경계 (UTM-K GeoJSON)",
		Params:      []Param{},
	},
	{
		Group: "boundary", Name: "grid-data", Method: "GET",
		Path:        "grid/data.geojson",
		Description: "행정구역 격자경계 (UTM-K GeoJSON)",
		Params:      []Param{},
	},
	{
		Group: "boundary", Name: "figure-buildingarea", Method: "GET",
		Path:        "figure/buildingarea.geojson",
		Description: "전개도 건물경계 (UTM-K GeoJSON)",
		Params:      []Param{},
	},
	{
		Group: "boundary", Name: "figure-floorboundary", Method: "GET",
		Path:        "figure/floorboundary.geojson",
		Description: "층별 최외각 공간속성 (UTM-K GeoJSON)",
		Params:      []Param{},
	},
	{
		Group: "boundary", Name: "figure-floorcompany", Method: "GET",
		Path:        "figure/floorcompany.geojson",
		Description: "층별 사업체 공간속성 (UTM-K GeoJSON)",
		Params:      []Param{},
	},

	// ── geocode — 지오코딩/좌표변환 ───────────────────────────────────────────
	{
		Group: "geocode", Name: "geocode", Method: "GET",
		Path:        "addr/geocode.json",
		Description: "지오코딩 (주소→UTM-K 좌표)",
		Params: []Param{
			{Name: "address", Required: true, Description: "검색 주소"},
			{Name: "pagenum", Required: false, Description: "페이지 번호 (default 0)"},
			{Name: "resultcount", Required: false, Description: "결과 수 (1~50, default 5)"},
		},
	},
	{
		Group: "geocode", Name: "geocodewgs84", Method: "GET",
		Path:        "addr/geocodewgs84.json",
		Description: "지오코딩(WGS84) (주소→WGS84 좌표)",
		Params: []Param{
			{Name: "address", Required: true, Description: "검색 주소"},
			{Name: "pagenum", Required: false, Description: "페이지 번호 (default 0)"},
			{Name: "resultcount", Required: false, Description: "결과 수 (1~50, default 5)"},
		},
	},
	{
		Group: "geocode", Name: "rgeocode", Method: "GET",
		Path:        "addr/rgeocode.json",
		Description: "리버스 지오코딩 (UTM-K 좌표→주소)",
		Params: []Param{
			{Name: "x_coor", Required: true, Description: "X 좌표 (UTM-K)"},
			{Name: "y_coor", Required: true, Description: "Y 좌표 (UTM-K)"},
		},
	},
	{
		Group: "geocode", Name: "rgeocodewgs84", Method: "GET",
		Path:        "addr/rgeocodewgs84.json",
		Description: "리버스 지오코딩(WGS84) (WGS84 좌표→주소)",
		Params: []Param{
			{Name: "x_coor", Required: true, Description: "경도 (WGS84)"},
			{Name: "y_coor", Required: true, Description: "위도 (WGS84)"},
		},
	},
	{
		Group: "geocode", Name: "transcoord", Method: "GET",
		Path:        "transformation/transcoord.json",
		Description: "좌표변환",
		Params: []Param{
			{Name: "src", Required: true, Description: "원본 좌표계 코드"},
			{Name: "dst", Required: true, Description: "대상 좌표계 코드"},
			{Name: "posX", Required: true, Description: "원본 X 좌표"},
			{Name: "posY", Required: true, Description: "원본 Y 좌표"},
		},
	},

	// ── code — 행정구역코드/기준연도 ──────────────────────────────────────────
	{
		Group: "code", Name: "stage", Method: "GET",
		Path:        "addr/stage.json",
		Description: "단계별 주소 조회 (시도/시군구/읍면동 코드)",
		Params: []Param{
			{Name: "cd", Required: false, Description: "상위 코드 (미지정=시도, 2자리=시군구, 5자리=읍면동)"},
			{Name: "pg_yn", Required: false, Description: "경량화 경계 포함 여부 (0/1)"},
		},
	},
	{
		Group: "code", Name: "findcodeinsmallarea", Method: "GET",
		Path:        "personal/findcodeinsmallarea.json",
		Description: "소지역 코드찾기",
		Params:      []Param{},
	},
	{
		Group: "code", Name: "year-data", Method: "GET",
		Path:        "year/data.json",
		Description: "최신/전체 기준년도 조회",
		Params:      []Param{},
	},
	{
		Group: "code", Name: "industrycode", Method: "GET",
		Path:        "stats/industrycode.json",
		Description: "산업분류 코드 (data 그룹과 공유)",
		Params: []Param{
			{Name: "class_deg", Required: true, Description: "산업분류 차수"},
			{Name: "class_code", Required: false, Description: "상위 분류코드"},
		},
	},

	// ── search — 검색(연관어/SOP) ──────────────────────────────────────────────
	{
		Group: "search", Name: "relword", Method: "GET",
		Path:        "search/relword.json",
		Description: "연관어검색 (검색어→유의어)",
		Params: []Param{
			{Name: "searchword", Required: true, Description: "검색어"},
		},
	},
	{
		Group: "search", Name: "sop", Method: "GET",
		Path:        "search/sop.json",
		Description: "SOP검색 (검색어→통계 SOP 정보)",
		Params: []Param{
			{Name: "searchword", Required: true, Description: "검색어"},
			{Name: "pagenum", Required: false, Description: "검색페이지 (default 0)"},
			{Name: "resultcount", Required: false, Description: "페이지당 결과 수 (1~50, default 5)"},
		},
	},
}

// EndpointsByGroup은 지정 그룹의 엔드포인트 슬라이스를 반환합니다.
func EndpointsByGroup(group string) []Endpoint {
	var result []Endpoint
	for _, ep := range Registry {
		if ep.Group == group {
			result = append(result, ep)
		}
	}
	return result
}

// FindEndpoint는 (group, name) 쌍으로 엔드포인트를 찾아 반환합니다.
func FindEndpoint(group, name string) (*Endpoint, bool) {
	for i := range Registry {
		if Registry[i].Group == group && Registry[i].Name == name {
			return &Registry[i], true
		}
	}
	return nil, false
}

// Groups는 Registry에 존재하는 고유 그룹명 목록을 반환합니다 (순서 보장).
func Groups() []string {
	seen := make(map[string]bool)
	var result []string
	for _, ep := range Registry {
		if !seen[ep.Group] {
			seen[ep.Group] = true
			result = append(result, ep.Group)
		}
	}
	return result
}
