package output

import (
	"encoding/json"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// renderXLSX writes result to an Excel file (.xlsx).
// opts.OutFile must be non-empty (xlsx to stdout is not meaningful).
// Sheet name is taken from opts.Title, falling back to "Sheet1".
//
// - Array of objects → header row + data rows (Fields projection applied).
// - Single object    → 2-column key/value sheet.
func renderXLSX(result json.RawMessage, isArray bool, opts Options) error {
	// opts.OutFile is validated in Render before we get here.
	sheetName := opts.Title
	if sheetName == "" {
		sheetName = "Sheet1"
	}

	wb := excelize.NewFile()
	defer wb.Close()

	// Rename the default sheet rather than creating a new one to avoid the
	// "workbook must contain at least one sheet" error during DeleteSheet.
	wb.SetSheetName("Sheet1", sheetName)

	// Bold + light-blue header style
	headerStyle, err := wb.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"ADD8E6"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return fmt.Errorf("헤더 스타일 생성 실패: %w", err)
	}

	if isArray {
		rows, err := parseArray(result)
		if err != nil {
			return fmt.Errorf("배열 파싱 실패: %w", err)
		}
		if err := writeXLSXArray(wb, sheetName, rows, opts.Fields, headerStyle); err != nil {
			return err
		}
	} else {
		obj, err := parseObject(result)
		if err != nil {
			return fmt.Errorf("객체 파싱 실패: %w", err)
		}
		if err := writeXLSXObject(wb, sheetName, obj, opts.Fields, headerStyle); err != nil {
			return err
		}
	}

	if err := wb.SaveAs(opts.OutFile); err != nil {
		return fmt.Errorf("XLSX 파일 저장 실패 (%s): %w", opts.OutFile, err)
	}
	return nil
}

func writeXLSXArray(wb *excelize.File, sheet string, rows []map[string]interface{}, fields []string, headerStyle int) error {
	if len(rows) == 0 {
		return nil
	}
	cols := projectFields(rows[0], fields)

	// Write headers
	for i, c := range cols {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		cell := colName + "1"
		if err := wb.SetCellValue(sheet, cell, c); err != nil {
			return fmt.Errorf("헤더 셀 쓰기 실패: %w", err)
		}
		if err := wb.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return fmt.Errorf("헤더 스타일 적용 실패: %w", err)
		}
	}

	// Write data
	for ri, row := range rows {
		for ci, c := range cols {
			colName, _ := excelize.ColumnNumberToName(ci + 1)
			cell := fmt.Sprintf("%s%d", colName, ri+2)
			if err := wb.SetCellValue(sheet, cell, formatValue(row[c])); err != nil {
				return fmt.Errorf("데이터 셀 쓰기 실패 (행 %d): %w", ri+1, err)
			}
		}
	}

	// Auto column width (15 chars default)
	for i := range cols {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = wb.SetColWidth(sheet, colName, colName, 15)
	}
	return nil
}

func writeXLSXObject(wb *excelize.File, sheet string, obj map[string]interface{}, fields []string, headerStyle int) error {
	// Header row: key | value
	for i, h := range []string{"key", "value"} {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		cell := colName + "1"
		if err := wb.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("헤더 셀 쓰기 실패: %w", err)
		}
		if err := wb.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return fmt.Errorf("헤더 스타일 적용 실패: %w", err)
		}
	}

	keys := projectFields(obj, fields)
	for ri, k := range keys {
		rowNum := ri + 2
		aCell := fmt.Sprintf("A%d", rowNum)
		bCell := fmt.Sprintf("B%d", rowNum)
		if err := wb.SetCellValue(sheet, aCell, k); err != nil {
			return fmt.Errorf("키 셀 쓰기 실패: %w", err)
		}
		if err := wb.SetCellValue(sheet, bCell, formatValue(obj[k])); err != nil {
			return fmt.Errorf("값 셀 쓰기 실패: %w", err)
		}
	}

	_ = wb.SetColWidth(sheet, "A", "A", 20)
	_ = wb.SetColWidth(sheet, "B", "B", 30)
	return nil
}
