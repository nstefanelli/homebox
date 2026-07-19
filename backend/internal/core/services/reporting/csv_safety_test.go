package reporting

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func TestSafeCSVText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "ordinary text", value: "Cordless drill", want: "Cordless drill"},
		{name: "equals formula", value: "=HYPERLINK(\"https://example.test\")", want: "'=HYPERLINK(\"https://example.test\")"},
		{name: "plus formula", value: "+SUM(1,2)", want: "'+SUM(1,2)"},
		{name: "minus formula", value: "-2+3", want: "'-2+3"},
		{name: "at formula", value: "@SUM(1,2)", want: "'@SUM(1,2)"},
		{name: "formula after spaces", value: "  =1+1", want: "'  =1+1"},
		{name: "formula after tab", value: "\t=1+1", want: "'\t=1+1"},
		{name: "leading tab", value: "\tplain", want: "'\tplain"},
		{name: "already escaped", value: "'=1+1", want: "'=1+1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SafeCSVText(tt.value))
		})
	}
}

func TestBillOfMaterialsCSV_NeutralizesFormulaText(t *testing.T) {
	out, err := BillOfMaterialsCSV([]repo.EntityOut{{
		EntitySummary: repo.EntitySummary{
			Name:        "=HYPERLINK(\"https://attacker.test\")",
			Description: "+cmd|' /C calc'!A0",
		},
		Manufacturer: "@SUM(1,2)",
		SerialNumber: "-2+3",
		ModelNumber:  "safe-model",
	}})
	require.NoError(t, err)

	records, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)

	header := records[0]
	row := records[1]
	values := make(map[string]string, len(header))
	for i := range header {
		values[header[i]] = row[i]
	}
	assert.Equal(t, "'=HYPERLINK(\"https://attacker.test\")", values["Name"])
	assert.Equal(t, "'+cmd|' /C calc'!A0", values["Description"])
	assert.Equal(t, "'@SUM(1,2)", values["Manufacturer"])
	assert.Equal(t, "'-2+3", values["Serial Number"])
	assert.Equal(t, "safe-model", values["Model Number"])
}

func TestIOSheetCSV_NeutralizesFormulaText(t *testing.T) {
	sheet := IOSheet{
		headers: []string{"HB.name", "HB.description", "HB.quantity", "HB.field.command"},
		Rows: []ExportCSVRow{{
			Name:        "=1+1",
			Description: "  @SUM(1,2)",
			Quantity:    -2,
			Fields: []ExportItemFields{{
				Name:  "command",
				Value: "+cmd|' /C calc'!A0",
			}},
		}},
	}

	rows, err := sheet.CSV()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "'=1+1", rows[1][0])
	assert.Equal(t, "'  @SUM(1,2)", rows[1][1])
	assert.Equal(t, "-2", rows[1][2], "numeric cells must remain numeric")
	assert.Equal(t, "'+cmd|' /C calc'!A0", rows[1][3])
}
