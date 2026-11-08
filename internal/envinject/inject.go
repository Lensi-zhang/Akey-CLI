package envinject

import (
	"runtime"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
)

const DefaultPrefix = "API"

// Build 返回替换目标 Profile 三个变量后的新环境副本。
func Build(base []string, profile config.Profile, key string) []string {
	return build(base, profile, key, runtime.GOOS == "windows")
}

func build(base []string, profile config.Profile, key string, foldNames bool) []string {
	prefix := prefixOrDefault(profile.EnvPrefix)
	keyName, baseURLName, modelName := variableNames(prefix)

	result := make([]string, 0, len(base)+3)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if sameName(name, keyName, foldNames) ||
			sameName(name, baseURLName, foldNames) ||
			sameName(name, modelName, foldNames) {
			continue
		}
		result = append(result, entry)
	}

	result = append(result,
		keyName+"="+key,
		baseURLName+"="+profile.BaseURL,
		modelName+"="+profile.Model,
	)
	return result
}

func sameName(left, right string, fold bool) bool {
	if fold {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func prefixOrDefault(prefix string) string {
	if prefix == "" {
		return DefaultPrefix
	}
	return prefix
}

func variableNames(prefix string) (key, baseURL, model string) {
	return prefix + "_KEY", prefix + "_BASE_URL", prefix + "_MODEL"
}
