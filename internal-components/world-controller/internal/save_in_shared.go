package internal

import (
	"os"
	"path/filepath"

	"github.com/m-szalik/goutils"
)

func SaveInShared(filename string, data []byte) error {
	dir := goutils.Env(SharedPathEnvVariable, "/tmp")
	fp := filepath.Join(dir, filename)
	fpTmp := fp + ".tmp"
	err := os.WriteFile(fpTmp, data, 0o744)
	if err != nil {
		return err
	}
	return os.Rename(fpTmp, fp)
}
