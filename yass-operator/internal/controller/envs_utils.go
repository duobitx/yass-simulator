package controller

import "strings"

func NormalizeEnvName(envName string) string {
	return strings.ReplaceAll(strings.ToUpper(envName), "-", "_")
}
