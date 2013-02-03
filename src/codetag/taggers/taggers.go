package taggers

import (
	"path/filepath"
	"os"
	"fmt"
	re "regexp"
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
var scm_paths = map[string]string{".git": "git", ".hg": "hg", ".bzr": "bzr", ".svn": "svn"}
func tagger_scm_detect_paths(name string, config *yaml.Node, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if !info.IsDir() {
		return nil
	}
	for dir, tag := range scm_paths {
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


type tag_pattern struct {
	pattern *re.Regexp
	tag string
}

var (
	// Only more-or-less plaintext (greppable) files for now
	lang_ext_map = map[string]string{//+as-is
		"py|tac": "py", "go": "go", "c(pp|xx)?|h": "c", "js|coffee": "js",
		"co?nf|cfg|ini": "conf", "unit|service|taget|mount|desktop|rules": "conf",
		"x?htm(l[45]?)?|css": "html", "xml|xsl": "xml",
		"patch|diff": "diff", "(ba|z|k|c|fi)?sh|env": "sh",
		"p(l|m)": "perl", "php[45]?": "php", "[ce]l|lisp": "lisp", "hs": "haskell",
		"md|markdown": "md", "rst": "rst",
		"ya?ml": "yaml", "json(\\.txt)?": "json", "do": "redo", "mk|a[cm]": "make" }//-as-is
	lang_path_map = map[string]string{//+as-is
		"/config$": "conf", "/Makefile$": "make", "/zsh/_[^/]+$": "sh", "patch": "diff" }//-as-is
	lang_regexps = []tag_pattern{}
)

func tagger_lang_detect_paths(name string, config *yaml.Node, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if info.IsDir() {
		return nil
	}
	for _, filter := range lang_regexps {
		if filter.pattern.Match([]byte(path)) {
			tags = append(tags, filter.tag)
		}
	}
	return
}


// map of available Tagger functions
var taggers = map[string]tagger_func {
	"scm_detect_paths": tagger_scm_detect_paths,
	"lang_detect_paths": tagger_lang_detect_paths,
}


func init() {
	// Compile patterns for tagger_lang_detect_paths
	for re_base, tag := range lang_ext_map {
		re_base = "\\.(" + re_base +
			")(\\.(default|in|(src-)?bak|backup|example|sample|dist|\\w+-new))?$"
		lang_regexps = append(lang_regexps, tag_pattern{re.MustCompile(re_base), tag})
	}
	for re_base, tag := range lang_path_map {
		lang_regexps = append(lang_regexps, tag_pattern{re.MustCompile(re_base), tag})
	}
}
