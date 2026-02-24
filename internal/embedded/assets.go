package embedded

import (
	"embed"
	"io/fs"
)

//go:embed all:web all:agent
var assetsFS embed.FS

func AssetsFS() fs.FS {
	return assetsFS
}

func WebFS() (fs.FS, error) {
	return fs.Sub(assetsFS, "web")
}

func AgentFS() (fs.FS, error) {
	return fs.Sub(assetsFS, "agent")
}
