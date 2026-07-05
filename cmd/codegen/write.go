package main

import (
	"os"
	"path/filepath"
)

func writeGeneratedFile(outputPath string, src []byte) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, src, 0o644)
}
