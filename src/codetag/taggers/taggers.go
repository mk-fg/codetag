package taggers

import (
	"os"
	"github.com/kylelemons/go-gypsy/yaml"
)

// Taggers are configurable plugins that return a string tag(s) for a file,
//  given it's location. What they do to that path (or files) is plugin-specific.
type Tagger struct {
	name string
	config *yaml.Node
}


// Initialize and return new Tagger object.
func Get(name string, config *yaml.Node) Tagger {
	return Tagger{name, config}
}

// Return tags that should be associated with the file/dir.
// Context value is passed to tagger plugins and
//  is basically an arbitrary map plugins can set values in.
// Context values are inherited along parent-child path relations - e.g.
//  if plugin sets {x: 1} for /foo, it'll see {x: 1} in /foo/bar, but not /bar or /bar/asd.
// "tags" key in context (sort.StringSlice) contains the tags that will be
//  applied to path in addition to what plugin will return and can be set/reset
//  by plugin itself or inherited from parent folder.
//  For example, "git" tag can be set once for directory that contains ".git"
//   path and will then be applied to all files within.
func (tagger *Tagger) GetTags(path string, info os.FileInfo, context *map[string]interface{}) []string {
	return nil
}
