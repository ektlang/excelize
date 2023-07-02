// Copyright 2016 - 2022 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to and
// read from XLAM / XLSM / XLSX / XLTM / XLTX files. Supports reading and
// writing spreadsheet documents generated by Microsoft Excel™ 2007 and later.
// Supports complex components by high compatibility, and provided streaming
// API for generating or reading data from a worksheet with huge amounts of
// data. This library needs Go version 1.15 or later.

package excelize

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// parseTableOptions provides a function to parse the format settings of the
// table with default value.
func parseTableOptions(opts string) (*tableOptions, error) {
	options := tableOptions{ShowRowStripes: true}
	err := json.Unmarshal(fallbackOptions(opts), &options)
	return &options, err
}

// AddTable provides the method to add table in a worksheet by given worksheet
// name, range reference and format set. For example, create a table of A1:D5
// on Sheet1:
//
//	err := f.AddTable("Sheet1", "A1", "D5", "")
//
// Create a table of F2:H6 on Sheet2 with format set:
//
//	err := f.AddTable("Sheet2", "F2", "H6", `{
//	    "table_name": "table",
//	    "table_style": "TableStyleMedium2",
//	    "show_first_column": true,
//	    "show_last_column": true,
//	    "show_row_stripes": false,
//	    "show_column_stripes": true
//	}`)
//
// Note that the table must be at least two lines including the header. The
// header cells must contain strings and must be unique, and must set the
// header row data of the table before calling the AddTable function. Multiple
// tables range reference that can't have an intersection.
//
// table_name: The name of the table, in the same worksheet name of the table should be unique
//
// table_style: The built-in table style names
//
//	TableStyleLight1 - TableStyleLight21
//	TableStyleMedium1 - TableStyleMedium28
//	TableStyleDark1 - TableStyleDark11
func (f *File) AddTable(sheet, hCell, vCell, opts string) error {
	options, err := parseTableOptions(opts)
	if err != nil {
		return err
	}
	// Coordinate conversion, convert C1:B3 to 2,0,1,2.
	hCol, hRow, err := CellNameToCoordinates(hCell)
	if err != nil {
		return err
	}
	vCol, vRow, err := CellNameToCoordinates(vCell)
	if err != nil {
		return err
	}

	if vCol < hCol {
		vCol, hCol = hCol, vCol
	}

	if vRow < hRow {
		vRow, hRow = hRow, vRow
	}

	tableID := f.countTables() + 1
	sheetRelationshipsTableXML := "../tables/table" + strconv.Itoa(tableID) + ".xml"
	tableXML := strings.ReplaceAll(sheetRelationshipsTableXML, "..", "xl")
	// Add first table for given sheet.
	sheetXMLPath, _ := f.getSheetXMLPath(sheet)
	sheetRels := "xl/worksheets/_rels/" + strings.TrimPrefix(sheetXMLPath, "xl/worksheets/") + ".rels"
	rID := f.addRels(sheetRels, SourceRelationshipTable, sheetRelationshipsTableXML, "")
	if err = f.addSheetTable(sheet, rID); err != nil {
		return err
	}
	f.addSheetNameSpace(sheet, SourceRelationship)
	if err = f.addTable(sheet, tableXML, hCol, hRow, vCol, vRow, tableID, options); err != nil {
		return err
	}
	return f.addContentTypePart(tableID, "table")
}

// countTables provides a function to get table files count storage in the
// folder xl/tables.
func (f *File) countTables() int {
	count := 0
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/tables/table") {
			count++
		}
		return true
	})
	return count
}

// addSheetTable provides a function to add tablePart element to
// xl/worksheets/sheet%d.xml by given worksheet name and relationship index.
func (f *File) addSheetTable(sheet string, rID int) error {
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return err
	}
	table := &xlsxTablePart{
		RID: "rId" + strconv.Itoa(rID),
	}
	if ws.TableParts == nil {
		ws.TableParts = &xlsxTableParts{}
	}
	ws.TableParts.Count++
	ws.TableParts.TableParts = append(ws.TableParts.TableParts, table)
	return err
}

// setTableHeader provides a function to set cells value in header row for the
// table.
func (f *File) setTableHeader(sheet string, x1, y1, x2 int) ([]*xlsxTableColumn, error) {
	var (
		tableColumns []*xlsxTableColumn
		idx          int
	)
	for i := x1; i <= x2; i++ {
		idx++
		cell, err := CoordinatesToCellName(i, y1)
		if err != nil {
			return tableColumns, err
		}
		name, _ := f.GetCellValue(sheet, cell)
		if _, err := strconv.Atoi(name); err == nil {
			_ = f.SetCellStr(sheet, cell, name)
		}
		if name == "" {
			name = "Column" + strconv.Itoa(idx)
			_ = f.SetCellStr(sheet, cell, name)
		}
		tableColumns = append(tableColumns, &xlsxTableColumn{
			ID:   idx,
			Name: name,
		})
	}
	return tableColumns, nil
}

// addTable provides a function to add table by given worksheet name,
// range reference and format set.
func (f *File) addTable(sheet, tableXML string, x1, y1, x2, y2, i int, opts *tableOptions) error {
	// Correct the minimum number of rows, the table at least two lines.
	if y1 == y2 {
		y2++
	}

	// Correct table range reference, such correct C1:B3 to B1:C3.
	ref, err := f.coordinatesToRangeRef([]int{x1, y1, x2, y2})
	if err != nil {
		return err
	}
	tableColumns, _ := f.setTableHeader(sheet, x1, y1, x2)
	name := opts.TableName
	if name == "" {
		name = "Table" + strconv.Itoa(i)
	}
	t := xlsxTable{
		XMLNS:       NameSpaceSpreadSheet.Value,
		ID:          i,
		Name:        name,
		DisplayName: name,
		Ref:         ref,
		AutoFilter: &xlsxAutoFilter{
			Ref: ref,
		},
		TableColumns: &xlsxTableColumns{
			Count:       len(tableColumns),
			TableColumn: tableColumns,
		},
		TableStyleInfo: &xlsxTableStyleInfo{
			Name:              opts.TableStyle,
			ShowFirstColumn:   opts.ShowFirstColumn,
			ShowLastColumn:    opts.ShowLastColumn,
			ShowRowStripes:    opts.ShowRowStripes,
			ShowColumnStripes: opts.ShowColumnStripes,
		},
	}
	table, _ := xml.Marshal(t)
	f.saveFileList(tableXML, table)
	return nil
}

// parseAutoFilterOptions provides a function to parse the settings of the auto
// filter.
func parseAutoFilterOptions(opts string) (*autoFilterOptions, error) {
	options := autoFilterOptions{}
	err := json.Unmarshal([]byte(opts), &options)
	return &options, err
}

// AutoFilter provides the method to add auto filter in a worksheet by given
// worksheet name, range reference and settings. An auto filter in Excel is a
// way of filtering a 2D range of data based on some simple criteria. For
// example applying an auto filter to a cell range A1:D4 in the Sheet1:
//
//	err := f.AutoFilter("Sheet1", "A1", "D4", "")
//
// Filter data in an auto filter:
//
//	err := f.AutoFilter("Sheet1", "A1", "D4", `{"column":"B","expression":"x != blanks"}`)
//
// column defines the filter columns in an auto filter range based on simple
// criteria
//
// It isn't sufficient to just specify the filter condition. You must also
// hide any rows that don't match the filter condition. Rows are hidden using
// the SetRowVisible() method. Excelize can't filter rows automatically since
// this isn't part of the file format.
//
// Setting a filter criteria for a column:
//
// expression defines the conditions, the following operators are available
// for setting the filter criteria:
//
//	==
//	!=
//	>
//	<
//	>=
//	<=
//	and
//	or
//
// An expression can comprise a single statement or two statements separated
// by the 'and' and 'or' operators. For example:
//
//	x <  2000
//	x >  2000
//	x == 2000
//	x >  2000 and x <  5000
//	x == 2000 or  x == 5000
//
// Filtering of blank or non-blank data can be achieved by using a value of
// Blanks or NonBlanks in the expression:
//
//	x == Blanks
//	x == NonBlanks
//
// Excel also allows some simple string matching operations:
//
//	x == b*      // begins with b
//	x != b*      // doesn't begin with b
//	x == *b      // ends with b
//	x != *b      // doesn't end with b
//	x == *b*     // contains b
//	x != *b*     // doesn't contains b
//
// You can also use '*' to match any character or number and '?' to match any
// single character or number. No other regular expression quantifier is
// supported by Excel's filters. Excel's regular expression characters can be
// escaped using '~'.
//
// The placeholder variable x in the above examples can be replaced by any
// simple string. The actual placeholder name is ignored internally so the
// following are all equivalent:
//
//	x     < 2000
//	col   < 2000
//	Price < 2000
func (f *File) AutoFilter(sheet, hCell, vCell, opts string) error {
	hCol, hRow, err := CellNameToCoordinates(hCell)
	if err != nil {
		return err
	}
	vCol, vRow, err := CellNameToCoordinates(vCell)
	if err != nil {
		return err
	}

	if vCol < hCol {
		vCol, hCol = hCol, vCol
	}

	if vRow < hRow {
		vRow, hRow = hRow, vRow
	}

	options, _ := parseAutoFilterOptions(opts)
	cellStart, _ := CoordinatesToCellName(hCol, hRow, true)
	cellEnd, _ := CoordinatesToCellName(vCol, vRow, true)
	ref, filterDB := cellStart+":"+cellEnd, "_xlnm._FilterDatabase"
	wb, err := f.workbookReader()
	if err != nil {
		return err
	}
	sheetID, err := f.GetSheetIndex(sheet)
	if err != nil {
		return err
	}
	filterRange := fmt.Sprintf("'%s'!%s", sheet, ref)
	d := xlsxDefinedName{
		Name:         filterDB,
		Hidden:       true,
		LocalSheetID: intPtr(sheetID),
		Data:         filterRange,
	}
	if wb.DefinedNames == nil {
		wb.DefinedNames = &xlsxDefinedNames{
			DefinedName: []xlsxDefinedName{d},
		}
	} else {
		var definedNameExists bool
		for idx := range wb.DefinedNames.DefinedName {
			definedName := wb.DefinedNames.DefinedName[idx]
			if definedName.Name == filterDB && *definedName.LocalSheetID == sheetID && definedName.Hidden {
				wb.DefinedNames.DefinedName[idx].Data = filterRange
				definedNameExists = true
			}
		}
		if !definedNameExists {
			wb.DefinedNames.DefinedName = append(wb.DefinedNames.DefinedName, d)
		}
	}
	refRange := vCol - hCol
	return f.autoFilter(sheet, ref, refRange, hCol, options)
}

// autoFilter provides a function to extract the tokens from the filter
// expression. The tokens are mainly non-whitespace groups.
func (f *File) autoFilter(sheet, ref string, refRange, col int, opts *autoFilterOptions) error {
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return err
	}
	if ws.SheetPr != nil {
		ws.SheetPr.FilterMode = true
	}
	ws.SheetPr = &xlsxSheetPr{FilterMode: true}
	filter := &xlsxAutoFilter{
		Ref: ref,
	}
	ws.AutoFilter = filter
	if opts.Column == "" || opts.Expression == "" {
		return nil
	}

	fsCol, err := ColumnNameToNumber(opts.Column)
	if err != nil {
		return err
	}
	offset := fsCol - col
	if offset < 0 || offset > refRange {
		return fmt.Errorf("incorrect index of column '%s'", opts.Column)
	}

	filter.FilterColumn = append(filter.FilterColumn, &xlsxFilterColumn{
		ColID: offset,
	})
	re := regexp.MustCompile(`"(?:[^"]|"")*"|\S+`)
	token := re.FindAllString(opts.Expression, -1)
	if len(token) != 3 && len(token) != 7 {
		return fmt.Errorf("incorrect number of tokens in criteria '%s'", opts.Expression)
	}
	expressions, tokens, err := f.parseFilterExpression(opts.Expression, token)
	if err != nil {
		return err
	}
	f.writeAutoFilter(filter, expressions, tokens)
	ws.AutoFilter = filter
	return nil
}

// writeAutoFilter provides a function to check for single or double custom
// filters as default filters and handle them accordingly.
func (f *File) writeAutoFilter(filter *xlsxAutoFilter, exp []int, tokens []string) {
	if len(exp) == 1 && exp[0] == 2 {
		// Single equality.
		var filters []*xlsxFilter
		filters = append(filters, &xlsxFilter{Val: tokens[0]})
		filter.FilterColumn[0].Filters = &xlsxFilters{Filter: filters}
	} else if len(exp) == 3 && exp[0] == 2 && exp[1] == 1 && exp[2] == 2 {
		// Double equality with "or" operator.
		var filters []*xlsxFilter
		for _, v := range tokens {
			filters = append(filters, &xlsxFilter{Val: v})
		}
		filter.FilterColumn[0].Filters = &xlsxFilters{Filter: filters}
	} else {
		// Non default custom filter.
		expRel := map[int]int{0: 0, 1: 2}
		andRel := map[int]bool{0: true, 1: false}
		for k, v := range tokens {
			f.writeCustomFilter(filter, exp[expRel[k]], v)
			if k == 1 {
				filter.FilterColumn[0].CustomFilters.And = andRel[exp[k]]
			}
		}
	}
}

// writeCustomFilter provides a function to write the <customFilter> element.
func (f *File) writeCustomFilter(filter *xlsxAutoFilter, operator int, val string) {
	operators := map[int]string{
		1:  "lessThan",
		2:  "equal",
		3:  "lessThanOrEqual",
		4:  "greaterThan",
		5:  "notEqual",
		6:  "greaterThanOrEqual",
		22: "equal",
	}
	customFilter := xlsxCustomFilter{
		Operator: operators[operator],
		Val:      val,
	}
	if filter.FilterColumn[0].CustomFilters != nil {
		filter.FilterColumn[0].CustomFilters.CustomFilter = append(filter.FilterColumn[0].CustomFilters.CustomFilter, &customFilter)
	} else {
		var customFilters []*xlsxCustomFilter
		customFilters = append(customFilters, &customFilter)
		filter.FilterColumn[0].CustomFilters = &xlsxCustomFilters{CustomFilter: customFilters}
	}
}

// parseFilterExpression provides a function to converts the tokens of a
// possibly conditional expression into 1 or 2 sub expressions for further
// parsing.
//
// Examples:
//
//	('x', '==', 2000) -> exp1
//	('x', '>',  2000, 'and', 'x', '<', 5000) -> exp1 and exp2
func (f *File) parseFilterExpression(expression string, tokens []string) ([]int, []string, error) {
	var expressions []int
	var t []string
	if len(tokens) == 7 {
		// The number of tokens will be either 3 (for 1 expression) or 7 (for 2
		// expressions).
		conditional := 0
		c := tokens[3]
		re, _ := regexp.Match(`(or|\|\|)`, []byte(c))
		if re {
			conditional = 1
		}
		expression1, token1, err := f.parseFilterTokens(expression, tokens[:3])
		if err != nil {
			return expressions, t, err
		}
		expression2, token2, err := f.parseFilterTokens(expression, tokens[4:7])
		if err != nil {
			return expressions, t, err
		}
		expressions = []int{expression1[0], conditional, expression2[0]}
		t = []string{token1, token2}
	} else {
		exp, token, err := f.parseFilterTokens(expression, tokens)
		if err != nil {
			return expressions, t, err
		}
		expressions = exp
		t = []string{token}
	}
	return expressions, t, nil
}

// parseFilterTokens provides a function to parse the 3 tokens of a filter
// expression and return the operator and token.
func (f *File) parseFilterTokens(expression string, tokens []string) ([]int, string, error) {
	operators := map[string]int{
		"==": 2,
		"=":  2,
		"=~": 2,
		"eq": 2,
		"!=": 5,
		"!~": 5,
		"ne": 5,
		"<>": 5,
		"<":  1,
		"<=": 3,
		">":  4,
		">=": 6,
	}
	operator, ok := operators[strings.ToLower(tokens[1])]
	if !ok {
		// Convert the operator from a number to a descriptive string.
		return []int{}, "", fmt.Errorf("unknown operator: %s", tokens[1])
	}
	token := tokens[2]
	// Special handling for Blanks/NonBlanks.
	re, _ := regexp.Match("blanks|nonblanks", []byte(strings.ToLower(token)))
	if re {
		// Only allow Equals or NotEqual in this context.
		if operator != 2 && operator != 5 {
			return []int{operator}, token, fmt.Errorf("the operator '%s' in expression '%s' is not valid in relation to Blanks/NonBlanks'", tokens[1], expression)
		}
		token = strings.ToLower(token)
		// The operator should always be 2 (=) to flag a "simple" equality in
		// the binary record. Therefore we convert <> to =.
		if token == "blanks" {
			if operator == 5 {
				token = " "
			}
		} else {
			if operator == 5 {
				operator = 2
				token = "blanks"
			} else {
				operator = 5
				token = " "
			}
		}
	}
	// If the string token contains an Excel match character then change the
	// operator type to indicate a non "simple" equality.
	re, _ = regexp.Match("[*?]", []byte(token))
	if operator == 2 && re {
		operator = 22
	}
	return []int{operator}, token, nil
}