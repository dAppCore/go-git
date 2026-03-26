package git

import (
	"path"

	"dappco.re/go/core"
)

func cleanPath(p string) string {
	if p == "" {
		return "."
	}
	return path.Clean(p)
}

func isAbsolutePath(p string) bool {
	return path.IsAbs(cleanPath(p))
}

func pathWithin(root, candidate string) bool {
	cleanRoot := cleanPath(root)
	cleanCandidate := cleanPath(candidate)

	if cleanRoot == "/" {
		return true
	}
	if cleanCandidate == cleanRoot {
		return true
	}

	return core.HasPrefix(cleanCandidate, core.Concat(cleanRoot, "/"))
}
