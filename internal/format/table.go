package format

import (
	"bytes"
	"strings"
	"text/tabwriter"
	"unicode"
)

// Table 渲染纯文本表格，不包含颜色或控制字符。
func Table(headers []string, rows [][]string) string {
	var buffer bytes.Buffer
	writer := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
	writeRow(writer, headers)
	for _, row := range rows {
		writeRow(writer, row)
	}
	_ = writer.Flush()
	return buffer.String()
}

func writeRow(writer *tabwriter.Writer, row []string) {
	for index, value := range row {
		if index > 0 {
			_, _ = writer.Write([]byte("\t"))
		}
		_, _ = writer.Write([]byte(sanitize(value)))
	}
	_, _ = writer.Write([]byte("\n"))
}

func sanitize(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, strings.ToValidUTF8(value, "�"))
}
