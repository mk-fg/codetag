package taggers

import (
	"path/filepath"
	"os"
	"fmt"
	"github.com/kylelemons/go-gypsy/yaml"
)


// Taggers are configurable routines that return a string tag(s) for a file,
//  given it's location. What they do to that path (or files) is plugin-specific.
type Tagger func(path string, info os.FileInfo, ctx *map[string]interface{}) []string


// Tagger before it is configured with "name" and "config".
// Should return tags that should be associated with the file/dir.
// Context value is passed to tagger plugins and
//  is basically an arbitrary map plugins can set values in.
// Context values are inherited along parent-child path relations - e.g.
//  if plugin sets {x: 1} for /foo, it'll see {x: 1} in /foo/bar, but not /bar or /bar/asd.
// "tags" key in context (sort.StringSlice) contains the tags that will be
//  applied to path in addition to what plugin will return and can be set/reset
//  by plugin itself or inherited from parent folder.
//  For example, "git" tag can be set once for directory that contains ".git"
//   path and will then be applied to all files within.
type tagger_func func(name string, config *yaml.Node, path string, info os.FileInfo, ctx *map[string]interface{}) []string


// Configure and return named "Tagger" function.
func Get(name string, config *yaml.Node) (Tagger, error) {
	tagger_func, ok := taggers[name]
	if !ok {
		return nil, fmt.Errorf("Unknown tagger type: %v", name)
	}
	tagger := func(path string, info os.FileInfo, ctx *map[string]interface{}) []string {
		return tagger_func(name, config, path, info, ctx)
	}
	return tagger, nil
}


// Assumes that there can be only one scm tag, so flushes previous tags if scm-path is detected.
var scm_paths = map[string]string{"git": ".git", "hg": ".hg", "svn": ".svn", "bzr": ".bzr"}
func tagger_scm_detect_paths(name string, config *yaml.Node, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if !info.IsDir() {
		return nil
	}
	for tag, dir := range scm_paths {
		info, err := os.Stat(filepath.Join(path, dir))
		if err == nil && info.IsDir() {
			if *ctx != nil {
				delete(*ctx, "tags")
			}
			tags = append(tags, tag)
		}
	}
	return
}


// map of available Tagger functions
var taggers = map[string]tagger_func {
	"scm_detect_paths": tagger_scm_detect_paths,
}
