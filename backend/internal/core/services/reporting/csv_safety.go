package reporting

import "strings"

// SafeCSVText neutralizes spreadsheet formulas in user-controlled text cells.
// CSV quoting alone does not stop Excel, Google Sheets, or LibreOffice from
// evaluating values that begin with formula sigils. Prefixing an apostrophe is
// the broadly-supported display-safe convention and leaves ordinary text
// untouched.
func SafeCSVText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}

	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	}

	switch value[0] {
	case '\t', '\r', '\n':
		return "'" + value
	}

	return value
}
