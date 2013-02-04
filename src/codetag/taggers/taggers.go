package taggers

import (
	"path/filepath"
	"os"
	"fmt"
	"strings"
	re "regexp"
	"github.com/mk-fg/go-logging"
	"github.com/vaughan0/go-ini"
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
type tagger_func func(name string, config interface{},
	log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) []string
type tagger_confproc func(name string, config *yaml.Node, log *logging.Logger) interface{}


// Configure and return named "Tagger" function.
func Get(name string, config *yaml.Node, log *logging.Logger) (Tagger, error) {
	// Config gets processed only once and passed to tagger as interface{}
	var tagger_conf interface{}
	tagger_conf = config
	tagger_confproc, ok := taggers_confproc[name]
	if ok {
		tagger_conf = tagger_confproc(name, config, log)
	}
	// Resulting Tagger is a closure created here
	tagger_func, ok := taggers[name]
	if !ok {
		return nil, fmt.Errorf("Unknown tagger type: %v", name)
	}
	tagger := func(path string, info os.FileInfo, ctx *map[string]interface{}) []string {
		return tagger_func(name, tagger_conf, log, path, info, ctx)
	}
	return tagger, nil
}


// Assumes that there can be only one scm tag, so flushes previous tags if scm-path is detected.
var scm_paths = map[string]string{".git": "git", ".hg": "hg", ".bzr": "bzr", ".svn": "svn"}
func tagger_scm_detect_paths(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
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


type path_tag_pattern struct {
	pattern *re.Regexp
	tag string
}

var (
	// Only more-or-less plaintext (greppable) files for now
	lang_ext_map = map[string]string{//+as-is
		`py|tac`: `py`, `go`: `go`, `c(pp|xx)?|h`: `c`, `js|coffee`: `js`,
		`co?nf|cfg|ini`: `conf`, `unit|service|taget|mount|desktop|rules`: `conf`,
		`x?htm(l[45]?)?|css`: `html`, `xml|xsl`: `xml`,
		`patch|diff`: `diff`, `(ba|z|k|c|fi)?sh|env`: `sh`,
		`p(l|m)`: `perl`, `php[45]?`: `php`, `[ce]l|lisp`: `lisp`, `hs`: `haskell`,
		`md|markdown`: `md`, `rst`: `rst`,
		`ya?ml`: `yaml`, `json(\.txt)?`: `json`, `do`: `redo`, `mk|a[cm]`: `make` }//-as-is
	lang_path_map = map[string]string{//+as-is
		`/config$`: `conf`, `/Makefile$`: `make`, `/zsh/_[^/]+$`: `sh`, `patch`: `diff` }//-as-is
	lang_regexps = []path_tag_pattern{}
)

func tagger_lang_detect_paths(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if info.IsDir() {
		return nil
	}
	for _, filter := range lang_regexps {
		if filter.pattern.MatchString(path) {
			tags = append(tags, filter.tag)
		}
	}
	return
}


var (//+as-is
	git_section_remote = re.MustCompile(`^\s*remote\s+"[^"]+"\s*$`)
	git_url_pattern = re.MustCompile(`^\s*` +
		`(git@|https?://([^:@]+(:[^@]+)?@)?)` + `(?P<host>[^:/]+)` + `(:|/)`)
)//-as-is

func tagger_scm_host_confproc(name string, config *yaml.Node, log *logging.Logger) interface{} {
	var err error
	config_map, ok := (*config).(yaml.Map)

	if !ok || len(config_map) == 0 {
		if len(config_map) == 0 {
			err = fmt.Errorf("no tags defined")
		} else {
			err = fmt.Errorf("invalid type - must be a map of tag:regexp")
		}
		log.Warnf("Error parsing tagger config (%v): %v", config, err)
		return nil
	}

	tag_map := make(map[string]*re.Regexp, len(config_map))
	for k, node := range config_map {
		pattern, ok := node.(yaml.Scalar)
		if !ok {
			log.Warnf("Failed to parse tag-host (tag: %v): %v", k, node)
			continue
		}
		regexp, err := re.Compile(strings.Trim(string(pattern), "'"))
		if err != nil {
			log.Warnf("Failed to parse tag-host pattern (%v: %v): %v", k, pattern, err)
			continue
		}
		tag_map[k] = regexp
	}

	return tag_map
}

func tagger_git_host(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if config == nil || !info.IsDir() {
		return
	}

	git_conf_path := filepath.Join(path, ".git/config")
	info, err := os.Stat(git_conf_path)
	if err != nil || info == nil || info.IsDir() {
		return
	}
	git_conf, err := ini.LoadFile(git_conf_path)
	if err != nil {
		log.Warnf("Failed to parse git config (%v): %v", git_conf_path, err)
		return
	}

	for header, section := range git_conf {
		if !git_section_remote.MatchString(header) {
			continue
		}
		for k, v := range section {
			if k != "url" {
				continue
			}
			host := string(git_url_pattern.ExpandString([]byte{},
				"${host}", v, git_url_pattern.FindStringSubmatchIndex(v)))
			tag_map, ok := config.(map[string]*re.Regexp)
			if !ok {
				panic(config)
			}
			for k, regexp := range tag_map {
				if regexp.MatchString(host) {
					// Can create duplicates, but it doesn't matter, since tags are de-duplicated on output/apply
					tags = append(tags, k)
				}
			}
		}
	}

	return
}


// map of available Tagger functions
var taggers = map[string]tagger_func {
	"scm_detect_paths": tagger_scm_detect_paths,
	"lang_detect_paths": tagger_lang_detect_paths,
	"git.host": tagger_git_host,
}
	// "host.bitbucket": tagger_host_bitbucket
var taggers_confproc = map[string]tagger_confproc {
	"git.host": tagger_scm_host_confproc,
}
	// "host.bitbucket": tagger_scm_host_confproc


func init() {
	// Compile patterns for tagger_lang_detect_paths
	for re_base, tag := range lang_ext_map {
		re_base = "\\.(" + re_base +
			")(\\.(default|in|(src-)?bak|backup|example|sample|dist|\\w+-new))?$"
		lang_regexps = append(lang_regexps, path_tag_pattern{re.MustCompile(re_base), tag})
	}
	for re_base, tag := range lang_path_map {
		lang_regexps = append(lang_regexps, path_tag_pattern{re.MustCompile(re_base), tag})
	}
}
