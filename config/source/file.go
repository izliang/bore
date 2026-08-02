package source

import (
	"os"
	"path/filepath"

	"github.com/izliang/bore/config"
)

// File creates a Source that reads a YAML configuration file.
// relPath is resolved relative to the project root directory
// (discovered via config.GetProjectRootDir).
//
// Example:
//
//	src := source.File("conf/config.yaml")
func File(relPath string) config.Source {
	return &fileSource{relPath: relPath}
}

type fileSource struct {
	relPath string
}

func (s *fileSource) Load() ([]byte, error) {
	root, err := config.GetProjectRootDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(root, s.relPath))
}
