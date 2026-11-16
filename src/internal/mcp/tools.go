package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/clazic/sgis/internal/api"
)

// MaxResultBytes는 도구 결과 텍스트 상한(약 200KB)입니다.
const MaxResultBytes = 200 * 1024

const truncNote = "잘림: 결과가 커서 앞부분만 담았어요. 범위를 좁히세요(adm_cd 를 더 하위 코드로, low_search 를 0/1 로)."

// field는 도구 입력 하나입니다. JSON Schema와 검증이 같은 정의를 씁니다.
type field struct {
	Name     string
	Type     string // "string" | "integer" | "number" | "boolean" | "array"(문자열 목록) | "object"
	Required bool
	Desc     string
	Pattern  string
	Enum     []string
	MaxLen   int
	Min, Max *float64
	Default  interface{}
}

// Tool은 MCP 도구 하나입니다.
type Tool struct {
	Name        string
	Title       string
	Description string
	Fields      []field
	// NoKeys면 API 키 없이 동작합니다(저장된 결과만 읽는 도구).
	NoKeys  bool
	Handler func(ctx context.Context, s *Server, cc *callCtx, args map[string]interface{}) (string, error)
}

func fptr(v float64) *float64 { return &v }

func (t *Tool) inputSchema() map[string]interface{} {
	props := map[string]interface{}{}
	required := []string{}
	for _, f := range t.Fields {
		p := map[string]interface{}{"type": f.Type, "description": f.Desc}
		if f.Type == "array" {
			p["items"] = map[string]interface{}{"type": "string", "maxLength": 100}
			p["maxItems"] = 100
		}
		if f.Pattern != "" {
			p["pattern"] = f.Pattern
		}
		if len(f.Enum) > 0 {
			p["enum"] = f.Enum
		}
		if f.MaxLen > 0 {
			p["maxLength"] = f.MaxLen
		}
		if f.Min != nil {
			p["minimum"] = *f.Min
		}
		if f.Max != nil {
			p["maximum"] = *f.Max
		}
		if f.Default != nil {
			p["default"] = f.Default
		}
		props[f.Name] = p
		if f.Required {
			required = append(required, f.Name)
		}
	}
	return map[string]interface{}{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

// validate는 인자를 검사하고 정규화된 값(string/float64/bool)을 돌려줍니다.
// 문자열 필드에 정수가 오면 문자열로 바꿔 받아 줍니다(예: year: 2020).
func (t *Tool) validate(in map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	known := map[string]bool{}
	for _, f := range t.Fields {
		known[f.Name] = true
	}
	var unknown []string
	for k := range in {
		if !known[k] {
			unknown = append(unknown, truncateRunes(k, 40))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("알 수 없는 인자: %s", strings.Join(unknown, ", "))
	}
	for _, f := range t.Fields {
		v, ok := in[f.Name]
		if !ok || v == nil {
			if f.Required {
				return nil, fmt.Errorf("%s 는 필수예요", f.Name)
			}
			if f.Default != nil {
				out[f.Name] = f.Default
			}
			continue
		}
		switch f.Type {
		case "string":
			var s string
			switch x := v.(type) {
			case string:
				s = strings.TrimSpace(x)
			case float64:
				if x != math.Trunc(x) || math.Abs(x) > 1e15 {
					return nil, fmt.Errorf("%s 는 문자열이어야 해요", f.Name)
				}
				s = strconv.FormatInt(int64(x), 10)
			default:
				return nil, fmt.Errorf("%s 는 문자열이어야 해요", f.Name)
			}
			if s == "" {
				if f.Required {
					return nil, fmt.Errorf("%s 는 필수예요", f.Name)
				}
				continue
			}
			maxLen := f.MaxLen
			if maxLen == 0 {
				maxLen = 100
			}
			if utf8.RuneCountInString(s) > maxLen {
				return nil, fmt.Errorf("%s 는 %d자 이하여야 해요", f.Name, maxLen)
			}
			if f.Pattern != "" && !regexp.MustCompile(f.Pattern).MatchString(s) {
				return nil, fmt.Errorf("%s 형식이 맞지 않아요(%s)", f.Name, f.Pattern)
			}
			if len(f.Enum) > 0 && !contains(f.Enum, s) {
				return nil, fmt.Errorf("%s 는 %s 중 하나여야 해요", f.Name, strings.Join(f.Enum, "|"))
			}
			out[f.Name] = s
		case "integer", "number":
			x, ok := v.(float64)
			if !ok {
				if sv, isStr := v.(string); isStr {
					p, err := strconv.ParseFloat(strings.TrimSpace(sv), 64)
					if err != nil {
						return nil, fmt.Errorf("%s 는 숫자여야 해요", f.Name)
					}
					x = p
				} else {
					return nil, fmt.Errorf("%s 는 숫자여야 해요", f.Name)
				}
			}
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, fmt.Errorf("%s 는 숫자여야 해요", f.Name)
			}
			if f.Type == "integer" && x != math.Trunc(x) {
				return nil, fmt.Errorf("%s 는 정수여야 해요", f.Name)
			}
			if f.Min != nil && x < *f.Min {
				return nil, fmt.Errorf("%s 는 %v 이상이어야 해요", f.Name, *f.Min)
			}
			if f.Max != nil && x > *f.Max {
				return nil, fmt.Errorf("%s 는 %v 이하여야 해요", f.Name, *f.Max)
			}
			out[f.Name] = x
		case "array":
			arr, ok := v.([]interface{})
			if !ok || len(arr) > 100 {
				return nil, fmt.Errorf("%s 는 문자열 목록(100개 이하)이어야 해요", f.Name)
			}
			list := make([]string, 0, len(arr))
			for _, it := range arr {
				sv, ok := it.(string)
				if !ok || utf8.RuneCountInString(sv) > 100 {
					return nil, fmt.Errorf("%s 는 문자열 목록이어야 해요", f.Name)
				}
				list = append(list, sv)
			}
			out[f.Name] = list
		case "object":
			m, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("%s 는 객체여야 해요", f.Name)
			}
			if b, _ := json.Marshal(m); len(b) > 10000 {
				return nil, fmt.Errorf("%s 가 너무 커요(10KB 이하)", f.Name)
			}
			out[f.Name] = m
		case "boolean":
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("%s 는 true/false 여야 해요", f.Name)
			}
			out[f.Name] = b
		}
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ── 공용 필드 ──────────────────────────────────────────────────────────────

const (
	patYear   = `^(19|20)\d{2}$`
	patAdmCd  = `^\d{2}(\d{3}(\d{3})?)?$`
	patCode   = `^[0-9A-Za-z_,.\-]{1,30}$`
	patEPSG   = `^\d{4,5}$`
	patKSIC   = `^[A-Za-z0-9]{1,10}$`
	patStageC = `^\d{2}(\d{3})?$`
)

func fYear(desc string) field {
	return field{Name: "year", Type: "string", Required: true, Pattern: patYear, MaxLen: 4,
		Desc: "기준연도(YYYY). " + desc + " 가용 연도는 sgis_years 로 확인."}
}

func fAdmCd(required bool) field {
	return field{Name: "adm_cd", Type: "string", Required: required, Pattern: patAdmCd, MaxLen: 8,
		Desc: "행정구역코드. 2자리=시도(11 서울), 5자리=시군구(11010 종로구), 8자리=읍면동. 코드는 sgis_admin_codes 로 찾기. 자릿수가 틀리면 조용히 빈 결과."}
}

func fLowSearch() field {
	return field{Name: "low_search", Type: "string", Enum: []string{"0", "1", "2"}, MaxLen: 1,
		Desc: "하위 단계 포함 — 0=해당 코드만, 1=1단계 하위(예: 시도→시군구별), 2=2단계 하위. adm_cd 와 둘 다 생략하면 자동으로 1(전국 시도별)."}
}

func fCode(name, desc string) field {
	return field{Name: name, Type: "string", Pattern: patCode, MaxLen: 30, Desc: desc + " (코드값은 SGIS 개발자 문서 참조)"}
}

// statsTool은 행정구역·연도 기반 통계 도구를 만듭니다(api.Registry 경로 재사용).
func statsTool(name, title, epName, desc, yearDesc string, extra ...field) *Tool {
	ep, ok := api.FindEndpoint("data", epName)
	if !ok {
		panic("endpoint missing: " + epName)
	}
	fields := append([]field{fYear(yearDesc), fAdmCd(false), fLowSearch()}, extra...)
	fields = append(fields, tableFields()...)
	return &Tool{
		Name: name, Title: title, Description: desc + " " + bigTableHint, Fields: fields,
		Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
			params := stringParams(a)
			if params["adm_cd"] == "" && params["low_search"] == "" {
				params["low_search"] = "1" // 둘 다 생략 시 errCd=-100 빈 결과 방지 (LEARNINGS)
			}
			raw, err := s.callAPI(ctx, cc, ep.Path, params)
			if err != nil {
				return "", err
			}
			return s.tableResult(cc, name, a, raw)
		},
	}
}

const bigTableHint = "큰 표(전국 읍면동 등)는 먼저 filter/aggregate 로 줄이고, 전체가 필요하면 download(resource_link)를 쓰세요. 결과가 약 200KB 를 넘으면 첫 쪽과 next_cursor 를 주며 sgis_result_page 로 다음 쪽을 받습니다."

// tableFields는 표를 내는 도구의 공통 선택 인자(형식·거르기·집계·저장)입니다.
func tableFields() []field {
	return []field{
		{Name: "format", Type: "string", Enum: []string{"json", "columns", "csv"},
			Desc: "결과 형식. 생략 시 작은 결과는 지금처럼 원본 JSON 배열, 큰 결과는 columns. json={요약, data:[객체]}, columns={요약, columns, rows, dict(adm_cd→adm_nm 등 이름은 한 번만)}, csv=첫 줄 요약 JSON + CSV"},
		{Name: "filter", Type: "object",
			Desc: `받은 뒤 거르기. {"열": ["값1","값2"]} 목록 안, {"열": {"from":x,"to":y}} 범위(숫자면 숫자 비교), {"열": "값"} 같음. 예: {"adm_cd":["11010","11020"]}`},
		{Name: "aggregate", Type: "object",
			Desc: `묶어 계산. {"group_by":["열",...], "op":"sum|avg|min|max|count", "value":"숫자 열"}. 숫자 아닌 값은 건너뛰고 skipped_non_numeric 로 알림`},
		{Name: "save", Type: "boolean", Desc: "true 면 작아도 결과를 저장해 result_id·download 를 줌"},
	}
}

// localArgs는 SGIS 로 보내지 않는 MCP 쪽 인자입니다.
var localArgs = map[string]bool{"format": true, "filter": true, "aggregate": true, "save": true,
	"properties": true, "decimals": true, "wgs84": true, "crs": true, "cursor": true, "limit": true, "result_id": true}

// stringParams는 문자열 인자를 API 쿼리로 옮깁니다(이름이 같음).
func stringParams(a map[string]interface{}) map[string]string {
	p := map[string]string{}
	for k, v := range a {
		if localArgs[k] {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			p[k] = s
		}
	}
	return p
}

func fmtNum(v interface{}) string {
	f, _ := v.(float64)
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func mustPath(group, name string) string {
	ep, ok := api.FindEndpoint(group, name)
	if !ok {
		panic("endpoint missing: " + group + "/" + name)
	}
	return ep.Path
}

func buildTools() []*Tool {
	stagePath := mustPath("code", "stage")
	yearPath := mustPath("code", "year-data")
	industryPath := mustPath("data", "industrycode")
	geocodePath := mustPath("geocode", "geocode")
	geocodeWGSPath := mustPath("geocode", "geocodewgs84")
	rgeocodePath := mustPath("geocode", "rgeocode")
	rgeocodeWGSPath := mustPath("geocode", "rgeocodewgs84")
	transPath := mustPath("geocode", "transcoord")
	hadmPath := mustPath("boundary", "hadmarea")

	return []*Tool{
		{
			Name: "sgis_admin_codes", Title: "행정구역 코드 조회",
			Description: "SGIS 행정구역 코드(시도·시군구·읍면동)를 단계별로 조회합니다. 다른 sgis_* 도구의 adm_cd 를 찾을 때 먼저 쓰세요. " +
				"cd 생략=시도 17개, 2자리 시도코드=그 시군구, 5자리 시군구코드=그 읍면동. 결과의 cd·addr_name 과 대표 좌표(x_coor/y_coor, UTM-K)를 돌려줍니다.",
			Fields: []field{
				{Name: "cd", Type: "string", Pattern: patStageC, MaxLen: 5, Desc: "상위 코드. 생략=시도, 2자리=시군구 목록, 5자리=읍면동 목록"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				return s.jsonCall(ctx, cc, stagePath, stringParams(a))
			},
		},
		{
			Name: "sgis_years", Title: "가용 기준연도",
			Description: "SGIS 통계에 쓸 수 있는 기준연도 목록을 조회합니다. 통계 조회 전에 year 값을 확인할 때 씁니다(인구·가구·주택 2015~2024, 사업체 2000~2024, 농가 5년 주기).",
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				return s.jsonCall(ctx, cc, yearPath, map[string]string{})
			},
		},
		statsTool("sgis_population", "인구통계", "population",
			"행정구역별 총인구·평균나이·인구밀도·노령화지수·총가구·총주택 등 인구 지표(인구주택총조사 기반)를 조회합니다. "+
				"adm_cd 로 지역 1건, adm_cd+low_search=1 로 그 아래 지역별 목록. adm_cd·low_search 둘 다 없으면 전국 시도별.",
			"2015~2024."),
		statsTool("sgis_search_population", "조건별 인구", "searchpopulation",
			"성별·나이유형·교육수준·혼인상태 조건으로 행정구역별 인구를 조회합니다(고령·청년 인구 등 정책 대상 인구 추정). 조건 생략 시 전체.",
			"2015~2024.",
			fCode("gender", "성별(0=전체, 1=남, 2=여)"), fCode("age_type", "나이 유형 코드"),
			fCode("edu_level", "교육수준 코드"), fCode("mrg_state", "혼인상태 코드")),
		statsTool("sgis_household", "가구통계", "household",
			"행정구역별 가구 수·가구원 수·평균 가구원 수를 조회합니다. household_type 으로 가구 유형(1인 가구 등)을 좁힐 수 있습니다.",
			"2015~2024.",
			fCode("household_type", "가구 유형 코드"), fCode("ocptn_type", "점유 형태 코드")),
		statsTool("sgis_house", "주택통계", "house",
			"행정구역별 주택 수를 조회합니다. 주택유형·건축연도·연면적 구간으로 세분할 수 있습니다(노후주택 비율 등).",
			"2015~2024.",
			fCode("house_type", "주택 유형 코드"), fCode("const_year", "건축연도 구간 코드"), fCode("house_area_cd", "주택 연면적 구간 코드")),
		statsTool("sgis_company", "사업체통계", "company",
			"행정구역별 사업체 수·종사자 수(전국사업체조사)를 조회합니다. class_code 로 업종(KSIC)을 좁히며, 코드는 sgis_industry_codes 로 찾습니다.",
			"2000~2024.",
			field{Name: "class_code", Type: "string", Pattern: patKSIC, MaxLen: 10, Desc: "산업분류코드(KSIC). sgis_industry_codes 로 조회"},
			fCode("theme_cd", "테마 코드")),
		statsTool("sgis_farm_household", "농가통계", "farmhousehold",
			"행정구역별 농가 수·농가 인구(농림어업총조사)를 조회합니다. 연도는 5년 주기(2000·2005·2010·2015·2020)만 됩니다.",
			"2000·2005·2010·2015·2020 만 가능."),
		{
			Name: "sgis_industry_codes", Title: "산업분류 코드",
			Description: "한국표준산업분류(KSIC) 코드를 조회합니다. sgis_company 의 class_code 를 찾을 때 씁니다. class_deg(차수)는 연도별로 다릅니다: 2000~2005=8, 2006~2016=9, 2017~2023=10, 2024~=11.",
			Fields: []field{
				{Name: "class_deg", Type: "string", Required: true, Pattern: `^\d{1,2}$`, MaxLen: 2, Desc: "산업분류 차수(8/9/10/11)"},
				{Name: "class_code", Type: "string", Pattern: patKSIC, MaxLen: 10, Desc: "상위 분류코드. 주면 그 하위 분류만"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				return s.jsonCall(ctx, cc, industryPath, stringParams(a))
			},
		},
		{
			Name: "sgis_geocode", Title: "주소→좌표",
			Description: "주소 문자열을 좌표로 바꿉니다(도로명·지번·부분 주소 가능). crs=wgs84(기본)면 경위도(x=경도, y=위도), utmk 면 UTM-K(EPSG:5179) 미터 좌표. 결과에 행정구역 코드도 들어 있어 adm_cd 찾기에 쓸 수 있습니다.",
			Fields: []field{
				{Name: "address", Type: "string", Required: true, MaxLen: 200, Desc: "검색 주소(예: \"서울특별시 종로구 세종대로 209\")"},
				{Name: "crs", Type: "string", Enum: []string{"wgs84", "utmk"}, Default: "wgs84", Desc: "출력 좌표계"},
				{Name: "pagenum", Type: "integer", Min: fptr(0), Max: fptr(1000), Desc: "페이지 번호(0부터)"},
				{Name: "resultcount", Type: "integer", Min: fptr(1), Max: fptr(50), Desc: "결과 수(1~50, 기본 5)"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				path := geocodeWGSPath
				if a["crs"] == "utmk" {
					path = geocodePath
				}
				p := map[string]string{"address": a["address"].(string)}
				for _, k := range []string{"pagenum", "resultcount"} {
					if v, ok := a[k]; ok {
						p[k] = fmtNum(v)
					}
				}
				return s.jsonCall(ctx, cc, path, p)
			},
		},
		{
			Name: "sgis_reverse_geocode", Title: "좌표→주소",
			Description: "좌표를 주소로 바꿉니다. crs=wgs84(기본)면 x=경도, y=위도; utmk 면 UTM-K(EPSG:5179) 미터 좌표.",
			Fields: []field{
				{Name: "x", Type: "number", Required: true, Desc: "경도(wgs84) 또는 UTM-K X"},
				{Name: "y", Type: "number", Required: true, Desc: "위도(wgs84) 또는 UTM-K Y"},
				{Name: "crs", Type: "string", Enum: []string{"wgs84", "utmk"}, Default: "wgs84", Desc: "입력 좌표계"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				x, y := a["x"].(float64), a["y"].(float64)
				path := rgeocodeWGSPath
				if a["crs"] == "utmk" {
					path = rgeocodePath
					if x < 0 || x > 3e6 || y < 0 || y > 4e6 {
						return "", errors.New("입력 오류: UTM-K 좌표 범위를 벗어났어요")
					}
				} else if x < 120 || x > 135 || y < 30 || y > 45 {
					return "", errors.New("입력 오류: 한국 경위도 범위(x 경도 120~135, y 위도 30~45)를 벗어났어요")
				}
				return s.jsonCall(ctx, cc, path, map[string]string{"x_coor": fmtNum(x), "y_coor": fmtNum(y)})
			},
		},
		{
			Name: "sgis_transform_coord", Title: "좌표계 변환",
			Description: "좌표 하나를 다른 좌표계로 바꿉니다(EPSG 코드, 예: 5179=UTM-K, 4326=WGS84, 5174/5186 등).",
			Fields: []field{
				{Name: "src", Type: "string", Required: true, Pattern: patEPSG, MaxLen: 5, Desc: "원본 EPSG 코드"},
				{Name: "dst", Type: "string", Required: true, Pattern: patEPSG, MaxLen: 5, Desc: "대상 EPSG 코드"},
				{Name: "x", Type: "number", Required: true, Desc: "원본 X(경도 또는 easting)"},
				{Name: "y", Type: "number", Required: true, Desc: "원본 Y(위도 또는 northing)"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				return s.jsonCall(ctx, cc, transPath, map[string]string{
					"src": a["src"].(string), "dst": a["dst"].(string),
					"posX": fmtNum(a["x"]), "posY": fmtNum(a["y"]),
				})
			},
		},
		{
			Name: "sgis_boundary", Title: "행정구역 경계 GeoJSON",
			Description: "행정구역(시도·시군구·읍면동) 경계를 GeoJSON FeatureCollection 텍스트로 돌려줍니다. " +
				"SGIS 원본 좌표는 UTM-K(EPSG:5179)이고, wgs84=true(기본)면 CLI 의 --wgs84 와 같은 방식으로 경위도로 재투영합니다. " +
				"좌표는 기본 wgs84 소수 6자리 / UTM-K 소수 1자리(decimals 로 조절), properties 로 속성을 고를 수 있습니다. " +
				"결과가 약 200KB 를 넘으면 feature 단위로 나눠 첫 쪽(유효한 GeoJSON, 맨 앞에 total_rows·next_cursor 등 요약 칸)과 download(.geojson resource_link)를 주고, sgis_result_page 로 다음 쪽을 받습니다. 지도용 전체 파일은 download 를 쓰세요. " +
				"면적·인구 등 수치만 필요하면 통계 도구를 쓰세요.",
			Fields: []field{
				{Name: "year", Type: "string", Required: true, Pattern: patYear, MaxLen: 4, Desc: "경계 기준연도(YYYY, 2000~2025)"},
				fAdmCd(true),
				{Name: "low_search", Type: "string", Enum: []string{"0", "1", "2"}, MaxLen: 1, Desc: "하위 단계 포함 — 0=해당 지역 경계만, 1=1단계 하위 경계들"},
				{Name: "wgs84", Type: "boolean", Default: true, Desc: "true=WGS84 경위도로 재투영(웹지도용), false=UTM-K 원본"},
				{Name: "decimals", Type: "integer", Min: fptr(0), Max: fptr(8), Desc: "좌표 소수 자릿수(기본 wgs84 6, UTM-K 1). 줄이면 작아짐(wgs84 5 ≈ 1m)"},
				{Name: "properties", Type: "array", Desc: "남길 feature 속성 이름 목록(예: [\"adm_cd\",\"adm_nm\"]). 생략=전부"},
				{Name: "format", Type: "string", Enum: []string{"geojson"}, Desc: "geojson(유일)"},
				{Name: "save", Type: "boolean", Desc: "true 면 작아도 저장해 result_id·download 를 줌"},
			},
			Handler: func(ctx context.Context, s *Server, cc *callCtx, a map[string]interface{}) (string, error) {
				p := stringParams(a)
				raw, err := s.callAPI(ctx, cc, hadmPath, p)
				if err != nil {
					return "", err
				}
				cc.report(fmt.Sprintf("GeoJSON %dKB 받음, 좌표 정리 중", len(raw)/1024))
				return s.geoResult(cc, "sgis_boundary", a, raw)
			},
		},
	}
}

// jsonCall은 API를 부르고 결과 JSON을 텍스트로 돌려줍니다(상한 초과 시 잘라 냄).
func (s *Server) jsonCall(ctx context.Context, cc *callCtx, path string, params map[string]string) (string, error) {
	raw, err := s.callAPI(ctx, cc, path, params)
	if err != nil {
		return "", err
	}
	return capText(compactJSON(raw), MaxResultBytes), nil
}

func compactJSON(raw json.RawMessage) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// capText는 텍스트를 limit 바이트 이하로 자르고 안내를 붙입니다(UTF-8 경계 보존).
func capText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit - len(truncNote) - 8
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n…\n" + truncNote
}

// roundCoords는 "coordinates" 배열의 숫자를 decimals 자리로 반올림합니다.
func roundCoords(v interface{}, decimals int) interface{} {
	pow := math.Pow(10, float64(decimals))
	var walk func(v interface{}, inCoords bool) interface{}
	walk = func(v interface{}, inCoords bool) interface{} {
		switch x := v.(type) {
		case map[string]interface{}:
			for k, vv := range x {
				x[k] = walk(vv, k == "coordinates")
			}
			return x
		case []interface{}:
			for i := range x {
				x[i] = walk(x[i], inCoords)
			}
			return x
		case float64:
			if inCoords {
				return math.Round(x*pow) / pow
			}
		}
		return v
	}
	return walk(v, false)
}
