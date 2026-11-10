package sitec

import (
	"os"
	"path/filepath"
)

// IsApp reports whether root is an application, as opposed to a module or its demo: it has a config/
// folder or a web/<binary>/main.go. An app is told what it lacks (an icon, a theme); a module has no
// icon or theme of its own and is never asked for one. One definition for every consumer.
func IsApp(root string) bool {
	if root == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(root, "config")); err == nil && info.IsDir() {
		return true
	}
	mains, _ := filepath.Glob(filepath.Join(root, "web", "*", "main.go"))
	return len(mains) > 0
}
