package format

import (
	"bytes"
	"encoding/json"
)

// JSON 将结果编码为稳定的单个 JSON 文档。
func JSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
