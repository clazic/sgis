// Package api는 SGIS Open API 엔드포인트 레지스트리와 HTTP 클라이언트를 제공합니다.
// 엔드포인트 정보는 references/sgis-endpoints.md 를 검토한 정적 정의입니다.
package api

// Param은 엔드포인트 파라미터 하나를 나타냅니다.
type Param struct {
	Name        string
	Required    bool
	Description string
}

// paramGlossary는 여러 엔드포인트에서 반복되는 공용 파라미터의 상세 설명입니다.
// 여기에 등록된 이름은 엔드포인트별 Description 대신 이 설명이 --help에 표시됩니다.
// 이름이 엔드포인트마다 뜻이 다른 파라미터(예: boundary userarea의 cd=단위 vs
// code stage의 cd=상위코드)는 충돌을 피해 등록하지 않고 엔드포인트 Description을 씁니다.
var paramGlossary = map[string]string{
	"year":            "기준연도(YYYY). 통계별 가용연도 상이 — 인구/가구/주택 2015~2024, 사업체 2000~2024, 농림어가 2000·2005·2010·2015·2020, 경계 2000~2025. 가용연도는 sgis code year-data로 확인",
	"adm_cd":          "행정구역코드. 미지정=전국 시도 목록, 2자리=시도(11 서울), 5자리=시군구(11010 종로구), 8자리=읍면동. 코드는 sgis code stage로 조회",
	"low_search":      "하위 단계 포함 범위 — 0=해당 코드만, 1=1단계 하위, 2=2단계 하위. adm_cd 미지정 시 이 값을 생략하면 결과가 비어(errCd=-100) 나올 수 있으니 --low-search 1(시도별) 등을 지정",
	"address":         "검색할 주소 문자열. 도로명·지번 모두 가능하며 부분 주소도 허용(예: \"서울특별시 종로구\")",
	"pagenum":         "결과 페이지 번호(0부터 시작, 기본 0)",
	"resultcount":     "페이지당 결과 수(1~50, 기본 5)",
	"searchword":      "검색어(통계 주제어, 예: 주택·인구)",
	"class_deg":       "산업분류 차수 — 연도별 상이: 2000~2005=8, 2006~2016=9, 2017~2023=10, 2024~=11",
	"src":             "원본 좌표계 EPSG 코드(예: 5179=UTM-K, 4326=WGS84)",
	"dst":             "대상 좌표계 EPSG 코드(예: 4326=WGS84, 5179=UTM-K)",
	"posX":            "변환할 원본 X 좌표값(src 좌표계 기준)",
	"posY":            "변환할 원본 Y 좌표값(src 좌표계 기준)",
	"consumer_key":    "SGIS 서비스 ID(개발자 포털 발급). 보통 sgis config로 관리하며 직접 지정 불필요",
	"consumer_secret": "SGIS 서비스 Secret(보안 Key). 보통 sgis config로 관리하며 직접 지정 불필요",
}

// ParamHelp는 파라미터 이름에 대한 상세 설명을 반환합니다.
// 공용 용어집에 등록된 이름이면 그 설명을, 아니면 fallback(엔드포인트별 설명)을 씁니다.
func ParamHelp(name, fallback string) string {
	if g, ok := paramGlossary[name]; ok {
		return g
	}
	return fallback
}

// Endpoint는 SGIS Open API 엔드포인트 하나를 나타냅니다.
// Path는 OpenAPI3 베이스(https://sgisapi.mods.go.kr/OpenAPI3/) 기준 상대 경로입니다.
// accessToken 파라미터는 Client가 자동으로 주입하므로 Params에 포함하지 않습니다.
type Endpoint struct {
	Group       string  // "data" | "boundary" | "geocode" | "code" | "auth"
	Name        string  // 명령 식별자 (slug)
	Path        string  // OpenAPI3 베이스 기준 상대 경로
	Method      string  // "GET" (현재 모든 SGIS 엔드포인트)
	Description string  // 한국어 설명 (--help Short)
	Long        string  // 상세 설명·사용 예시 (--help 본문, 선택)
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
		Long: `행정구역별 총인구·평균나이·인구밀도·노령화지수·총가구·총주택 등 인구 지표를 조회합니다.

조회 단위 정하기:
  --adm-cd 지정          해당 지역 1건 (예: --adm-cd 11 → 서울 전체)
  --adm-cd 생략 + --low-search 1   전국을 시도별로 (17개 행)
  --adm-cd 11 + --low-search 1     서울 아래 시군구별로

주의: --adm-cd 와 --low-search 를 둘 다 생략하면 결과가 비어(errCd=-100) 나옵니다.

예시:
  sgis data population --year 2024 --adm-cd 11
  sgis data population --year 2024 --low-search 1 -f csv -o pop.csv`,
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
		Long: `성별·나이·교육수준·혼인상태 등 조건으로 인구를 검색합니다.
population과 달리 인구 세부 속성으로 필터링할 때 사용합니다.
(gender/age_type/edu_level/mrg_state 코드값은 SGIS 개발자 문서 참조)

예시:
  sgis data searchpopulation --year 2020 --adm-cd 11 --gender 1`,
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "gender", Required: false, Description: "성별 (코드값은 SGIS 문서 참조)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "age_type", Required: false, Description: "나이 유형 (코드값은 SGIS 문서 참조)"},
			{Name: "edu_level", Required: false, Description: "교육수준 (코드값은 SGIS 문서 참조)"},
			{Name: "mrg_state", Required: false, Description: "혼인상태 (코드값은 SGIS 문서 참조)"},
		},
	},
	{
		Group: "data", Name: "household", Method: "GET",
		Path:        "stats/household.json",
		Description: "가구통계",
		Long: `행정구역별 가구 수·가구원 수·평균 가구원 수를 조회합니다.
조회 단위(adm_cd/low_search) 규칙은 population과 동일합니다.

예시:
  sgis data household --year 2020 --adm-cd 11 -f json`,
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "household_type", Required: false, Description: "가구 유형 (코드값은 SGIS 문서 참조)"},
			{Name: "ocptn_type", Required: false, Description: "직업 유형 (코드값은 SGIS 문서 참조)"},
		},
	},
	{
		Group: "data", Name: "house", Method: "GET",
		Path:        "stats/house.json",
		Description: "주택통계",
		Long: `행정구역별 주택 수를 조회합니다. 주택유형·건축연도·면적으로 세분할 수 있습니다.
조회 단위(adm_cd/low_search) 규칙은 population과 동일합니다.

예시:
  sgis data house --year 2020 --adm-cd 11 --house-type 1`,
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "house_type", Required: false, Description: "주택 유형 (코드값은 SGIS 문서 참조)"},
			{Name: "const_year", Required: false, Description: "건축연도 (YYYY 또는 구간코드; SGIS 문서 참조)"},
			{Name: "house_area_cd", Required: false, Description: "주택면적 구간코드 (SGIS 문서 참조)"},
		},
	},
	{
		Group: "data", Name: "company", Method: "GET",
		Path:        "stats/company.json",
		Description: "사업체통계",
		Long: `행정구역별 사업체 수·종사자 수를 조회합니다(연도 2000~2024).
산업분류코드(--class-code)로 업종을 좁힐 수 있으며, 코드는 industrycode로 조회합니다.
조회 단위(adm_cd/low_search) 규칙은 population과 동일합니다.

예시:
  sgis data company --year 2023 --adm-cd 11 --low-search 1 -f csv`,
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도 (2000~2024)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "class_code", Required: false, Description: "산업분류코드(KSIC). sgis data industrycode로 조회"},
			{Name: "theme_cd", Required: false, Description: "테마코드 (SGIS 문서 참조)"},
		},
	},
	{
		Group: "data", Name: "industrycode", Method: "GET",
		Path:        "stats/industrycode.json",
		Description: "산업분류코드",
		Long: `한국표준산업분류(KSIC) 코드를 조회합니다. company의 --class-code 값을 찾을 때 씁니다.
차수(--class-deg)는 연도에 따라 다릅니다: 2017~2023=10, 2024~=11.
--class-code 에 상위 분류를 지정하면 그 하위 분류만 반환합니다.

예시:
  sgis data industrycode --class-deg 10`,
		Params: []Param{
			{Name: "class_deg", Required: true, Description: "산업분류 차수"},
			{Name: "class_code", Required: false, Description: "상위 분류코드 지정 시 그 하위 분류만 반환"},
		},
	},
	{
		Group: "data", Name: "farmhousehold", Method: "GET",
		Path:        "stats/farmhousehold.json",
		Description: "농가통계",
		Long: `행정구역별 농가 수·농가 인구를 조회합니다.
가용연도는 5년 주기(2000·2005·2010·2015·2020)이며, 이 통계는 low_search 기본이 0입니다.

예시:
  sgis data farmhousehold --year 2020 --adm-cd 11 --low-search 1`,
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
		Long: `행정구역별 임가(임업 가구) 수·인구를 조회합니다.
가용연도는 5년 주기(2000·2005·2010·2015·2020)입니다.

예시:
  sgis data forestryhousehold --year 2020 --adm-cd 11`,
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
		Long: `행정구역별 어가(어업 가구) 수·인구를 조회합니다.
어업 구분(--oga-div)이 필수입니다(내수면/해수면 등, 코드값은 SGIS 문서 참조).
가용연도는 5년 주기(2000·2005·2010·2015·2020)입니다.

예시:
  sgis data fisheryhousehold --year 2020 --oga-div 1 --adm-cd 11`,
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "oga_div", Required: true, Description: "어업 구분 (예: 내수면/해수면; 코드값은 SGIS 문서 참조)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
		},
	},
	{
		Group: "data", Name: "householdmember", Method: "GET",
		Path:        "stats/householdmember.json",
		Description: "가구원통계",
		Long: `행정구역별 가구원(가구에 속한 개인) 인구를 조회합니다.
데이터 유형(--data-type)이 필수이며, 성별과 연령 구간을 --age-from 과 --age-to 로 좁힐 수 있습니다.

예시:
  sgis data householdmember --year 2020 --data-type 1 --adm-cd 11 --gender 2`,
		Params: []Param{
			{Name: "year", Required: true, Description: "기준연도"},
			{Name: "data_type", Required: true, Description: "데이터 유형 (코드값은 SGIS 문서 참조)"},
			{Name: "adm_cd", Required: false, Description: "행정구역코드"},
			{Name: "low_search", Required: false, Description: "하위 단계 포함"},
			{Name: "gender", Required: false, Description: "성별 (코드값은 SGIS 문서 참조)"},
			{Name: "age_from", Required: false, Description: "나이 구간 시작(세)"},
			{Name: "age_to", Required: false, Description: "나이 구간 끝(세)"},
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
		Long: `행정구역(시도/시군구/읍면동) 경계를 GeoJSON으로 조회합니다.
좌표계는 UTM-K(EPSG:5179)이며, Leaflet 등 웹지도에는 --wgs84 로 재투영해 씁니다.
--year 와 --adm-cd 가 모두 필수입니다.

예시:
  sgis boundary hadmarea --year 2024 --adm-cd 11 -o seoul.geojson
  sgis boundary hadmarea --year 2024 --adm-cd 11 --wgs84 -o seoul_wgs84.geojson`,
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
		Long: `통계 집계구(최소 통계 단위) 경계를 GeoJSON으로 조회합니다.
--adm-cd 는 8자리 읍면동 코드를 넣어야 그 안의 집계구가 반환됩니다.

예시:
  sgis boundary statsarea --adm-cd 11010530 -o jgg.geojson`,
		Params: []Param{
			{Name: "adm_cd", Required: true, Description: "행정구역코드 (8자리 읍면동)"},
		},
	},
	{
		Group: "boundary", Name: "userarea", Method: "GET",
		Path:        "boundary/userarea.geojson",
		Description: "영역내경계 (UTM-K GeoJSON)",
		Long: `사각형 영역(bounding box) 안에 걸치는 행정구역 경계를 GeoJSON으로 조회합니다.
--minx --miny --maxx --maxy 는 UTM-K(EPSG:5179) 미터 좌표이고,
--cd 로 반환 단위(시도/시군구/읍면동/집계구)를 정합니다.

예시:
  sgis boundary userarea --minx 953000 --miny 1951000 --maxx 955000 --maxy 1953000 --cd 2`,
		Params: []Param{
			{Name: "minx", Required: true, Description: "영역 최소 X 좌표 (UTM-K EPSG:5179, 미터)"},
			{Name: "miny", Required: true, Description: "영역 최소 Y 좌표 (UTM-K EPSG:5179, 미터)"},
			{Name: "maxx", Required: true, Description: "영역 최대 X 좌표 (UTM-K EPSG:5179, 미터)"},
			{Name: "maxy", Required: true, Description: "영역 최대 Y 좌표 (UTM-K EPSG:5179, 미터)"},
			{Name: "cd", Required: true, Description: "반환 단위 (1=시도, 2=시군구, 3=읍면동, 4=집계구)"},
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
		Long: `주소 문자열을 UTM-K(EPSG:5179) 좌표로 변환합니다.
Leaflet 등 웹지도용 위경도가 필요하면 geocodewgs84를 쓰세요.

예시:
  sgis geocode geocode --address "서울특별시 종로구 세종대로 209"`,
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
		Long: `주소 문자열을 WGS84(EPSG:4326) 위경도로 변환합니다. 웹지도(Leaflet/구글맵)용.
출력 X=경도, Y=위도입니다.

예시:
  sgis geocode geocodewgs84 --address "부산광역시 해운대구"`,
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
		Long: `UTM-K(EPSG:5179) 좌표를 주소로 역변환합니다.
입력 좌표가 위경도라면 rgeocodewgs84를 쓰세요.

예시:
  sgis geocode rgeocode --x-coor 953932 --y-coor 1952053`,
		Params: []Param{
			{Name: "x_coor", Required: true, Description: "X 좌표 (UTM-K EPSG:5179, 미터)"},
			{Name: "y_coor", Required: true, Description: "Y 좌표 (UTM-K EPSG:5179, 미터)"},
		},
	},
	{
		Group: "geocode", Name: "rgeocodewgs84", Method: "GET",
		Path:        "addr/rgeocodewgs84.json",
		Description: "리버스 지오코딩(WGS84) (WGS84 좌표→주소)",
		Long: `WGS84(EPSG:4326) 위경도를 주소로 역변환합니다.
--x-coor 에 경도, --y-coor 에 위도를 넣습니다.

예시:
  sgis geocode rgeocodewgs84 --x-coor 126.9784 --y-coor 37.5665`,
		Params: []Param{
			{Name: "x_coor", Required: true, Description: "경도 (WGS84 EPSG:4326)"},
			{Name: "y_coor", Required: true, Description: "위도 (WGS84 EPSG:4326)"},
		},
	},
	{
		Group: "geocode", Name: "transcoord", Method: "GET",
		Path:        "transformation/transcoord.json",
		Description: "좌표변환",
		Long: `한 좌표계의 좌표를 다른 좌표계로 변환합니다(예: UTM-K→WGS84).
--src 와 --dst 는 EPSG 코드(5179=UTM-K, 4326=WGS84 등)입니다.

예시:
  sgis geocode transcoord --src 5179 --dst 4326 --posX 953932 --posY 1952053`,
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
		Long: `행정구역 코드를 단계별로 조회합니다. 다른 명령의 --adm-cd 값을 찾을 때 씁니다.
--cd 를 생략하면 시도 17개, 시도코드(2자리)를 주면 그 시군구, 시군구코드(5자리)를 주면 읍면동을 반환합니다.

예시:
  sgis code stage              # 시도 목록
  sgis code stage --cd 11      # 서울 시군구
  sgis code stage --cd 11010   # 종로구 읍면동`,
		Params: []Param{
			{Name: "cd", Required: false, Description: "상위 코드 (미지정=시도, 2자리=시군구, 5자리=읍면동)"},
			{Name: "pg_yn", Required: false, Description: "경량화 경계(pg) 포함 여부 (0=제외, 1=포함)"},
		},
	},
	{
		Group: "code", Name: "findcodeinsmallarea", Method: "GET",
		Path:        "personal/findcodeinsmallarea.json",
		Description: "소지역 코드찾기",
		Long: `소지역(집계구) 코드를 검색합니다. 고정 파라미터는 없으며,
필요한 검색 조건은 --param key=value 로 전달합니다.`,
		Params: []Param{},
	},
	{
		Group: "code", Name: "year-data", Method: "GET",
		Path:        "year/data.json",
		Description: "최신/전체 기준년도 조회",
		Long: `통계에 사용 가능한 기준연도 목록을 조회합니다.
population 등 --year 에 넣을 수 있는 연도를 확인할 때 씁니다.

예시:
  sgis code year-data`,
		Params: []Param{},
	},
	{
		Group: "code", Name: "industrycode", Method: "GET",
		Path:        "stats/industrycode.json",
		Description: "산업분류 코드 (data 그룹과 공유)",
		Long: `한국표준산업분류(KSIC) 코드를 조회합니다. sgis data industrycode 와 동일합니다.
차수(--class-deg)는 연도별로 다릅니다: 2017~2023=10, 2024~=11.

예시:
  sgis code industrycode --class-deg 10`,
		Params: []Param{
			{Name: "class_deg", Required: true, Description: "산업분류 차수"},
			{Name: "class_code", Required: false, Description: "상위 분류코드 지정 시 그 하위 분류만 반환"},
		},
	},

	// ── search — 검색(연관어/SOP) ──────────────────────────────────────────────
	{
		Group: "search", Name: "relword", Method: "GET",
		Path:        "search/relword.json",
		Description: "연관어검색 (검색어→유의어)",
		Long: `검색어와 연관된 통계 유의어 목록을 조회합니다.
sop 검색 전에 적절한 검색어를 찾을 때 유용합니다.

예시:
  sgis search relword --searchword 주택`,
		Params: []Param{
			{Name: "searchword", Required: true, Description: "검색어"},
		},
	},
	{
		Group: "search", Name: "sop", Method: "GET",
		Path:        "search/sop.json",
		Description: "SOP검색 (검색어→통계 SOP 정보)",
		Long: `검색어로 통계 SOP(통계표) 정보를 조회합니다(통계 ID·기준연도·명칭·URL 반환).
결과가 많으면 --pagenum 과 --resultcount 로 페이지를 조절합니다.

예시:
  sgis search sop --searchword 주택 --resultcount 10`,
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
