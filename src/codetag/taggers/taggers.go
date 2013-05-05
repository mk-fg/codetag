package taggers

import (
	"path/filepath"
	"os"
	"fmt"
	"io"
	"bufio"
	"strings"
	re "regexp"
	"github.com/vaughan0/go-logging"
	"github.com/vaughan0/go-ini"
	"github.com/kylelemons/go-gypsy/yaml"
)


// Taggers are configurable routines that return a string tag(s) for a file,
//  given it's location. What they do to that path (or files) is plugin-specific.
type Tagger func(path string, info os.FileInfo, ctx *map[string]interface{}) []string

// Used to keep set of tags as keys.
type CtxTagset map[string]bool

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
	// Check if tagger should only be used as a fallback
	tagger_fallback := false
	if config != nil {
		node, err := yaml.Child(*config, "fallback")
		if err == nil {
			val, ok := node.(yaml.Scalar)
			if ok && string(val) == "true" {
				tagger_fallback = true
			}
		}
	}
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
	tagger := func(path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
		// Check fallback condition
		if tagger_fallback {
			tags_prev_if, ok := (*ctx)["tags"]
			if ok {
				tags_prev, ok := tags_prev_if.(CtxTagset)
				if !ok {
					panic(fmt.Errorf("Failed to process tags from context: %v", *ctx))
				}
				if len(tags_prev) > 0 {
					return
				}
			}
		}
		return tagger_func(name, tagger_conf, log, path, info, ctx)
	}
	return tagger, nil
}


// Assumes that there can be only one scm tag, so flushes previous tags if scm-path is detected.
var scm_paths = map[string]string{".git": "git", ".hg": "hg", ".bzr": "bzr", ".svn": "svn"}
func tagger_scm_detect_paths(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if !info.IsDir() {
		return
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
	lang_ext_map = map[string]string{
		`py|tac`: `py`, `go`: `go`, `c(c|pp|xx|\+\+)?|hh?|lex|y(acc)?`: `c`,
		`js(o?n(\.txt)?)?|coffee`: `js`, `co?nf|cf|cfg|ini`: `conf`,
		`unit|service|taget|mount|desktop|rules`: `conf`,
		`[sx]?htm(l[45]?)?|css|less`: `html`, `x[ms]l|xsd|dbk`: `xml`,
		`kml`: `kml`, `sgml|dtd`: `sgml`,
		`patch|diff|pat`: `diff`, `(ba|z|k|c|fi)?sh|env|exheres-\d+|ebuild|initd?`: `sh`, `sql`: `sql`,
		`p(l|m|erl|od)|al`: `perl`, `ph(p[s45t]?|tml)`: `php`, `[cejm]l|li?sp|rkt|sc[mh]|stk|ss`: `lisp`,
		`hs`: `haskell`, `rb`: `ruby`, `lua`: `lua`, `awk`: `awk`, `tcl`: `tcl`, `java`: `java`,
		`(?i)mk?d|markdown`: `md`, `re?st`: `rst`, `rdf`: `rdf`, `xul`: `xul`, `po`: `po`, `csv`: `csv`,
		`f(or)?`: `fortran`, `p(as)?`: `pascal`, `dpr`: `delphi`, `ad[abs]|ad[bs].dg`: `ada`,
		`ya?ml`: `yaml`, `jso?n(\.txt)?`: `json`, `do`: `redo`, `m[k4c]|a[cm]|cmake`: `make` }
	lang_path_map = map[string]string{
		`rakefile`: `ruby`, `/(Makefile|CMakeLists.txt|Imakefile|makepp|configure)$`: `make`,
		`/config$`: `conf`, `/zsh/_[^/]+$`: `sh`, `patch`: `diff` }
	lang_path_regexps = []path_tag_pattern{}
	lang_interpreter_map = map[string]string{
		`lua`: `lua`, `php\d?`: `php`,
		`j?ruby(\d\.\d)?|rbx`: `ruby`,
		`[jp]ython(\d(\.\d)?)?`: `py`,
		`[gnm]?awk`: `awk`,
		`(mini)?perl(\d(\.\d+)?)?`: `perl`,
		`wishx?|tcl(sh)?`: `tcl`,
		`scm|guile|clisp|racket|(sb)?cl|emacs`: `lisp`,
		`([bo]?a|t?c|k|z)?sh`: `sh` }
	lang_shebang = re.MustCompile(`^#!((/usr/bin/env)?\s+)?(?P<interpreter>\S+)`)
	lang_shebang_regexps = []path_tag_pattern{}
)

func tagger_lang_detect_paths(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if info.Mode() & os.ModeType != 0 {
		return
	}
	for _, filter := range lang_path_regexps {
		if filter.pattern.MatchString(path) {
			tags = append(tags, filter.tag)
		}
	}
	return
}

func tagger_lang_detect_shebang(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if info.Mode() & os.ModeType != 0 {
		return
	}

	src, err := os.Open(path)
	if err != nil {
		log.Infof("Failed to open file (%v): %v", path, err)
		return
	}
	defer src.Close()
	src_r := bufio.NewReader(src)
	line, err := src_r.ReadString('\n')
	if err != nil {
		if err != io.EOF {
			log.Warnf("Failed to read first line from file (%v): %v", path, err)
		}
		return
	}

	interpreter := string(lang_shebang.ExpandString([]byte{},
		"${interpreter}", line, lang_shebang.FindStringSubmatchIndex(line)))
	interpreter = filepath.Base(interpreter)
	for _, filter := range lang_shebang_regexps {
		if filter.pattern.MatchString(interpreter) {
			tags = append(tags, filter.tag)
		}
	}

	return
}


var (
	git_section_remote = re.MustCompile(`^\s*remote\s+"[^"]+"\s*$`)
	git_url_pattern = re.MustCompile(`^\s*` +
		`(git@|https?://([^:@]+(:[^@]+)?@)?)` + `(?P<host>[^:/]+)` + `(:|/)`)
	hg_url_pattern = re.MustCompile(`^\s*` +
		`https?://([^:@]+(:[^@]+)?@)?` + `(?P<host>[^:/]+)` + `/`)
)

func tagger_scm_host_confproc(name string, config *yaml.Node, log *logging.Logger) interface{} {
	var err error

	node, err := yaml.Child(*config, "host_tags")
	config_map, ok := yaml.Map{}, false
	if err == nil {
		config_map, ok = node.(yaml.Map)
	}

	if !ok || len(config_map) == 0 {
		if ok && len(config_map) == 0 {
			err = fmt.Errorf("no tags defined")
		} else {
			err = fmt.Errorf("must be a map of tag:regexp")
		}
		log.Warnf("Error parsing 'host_tags' in tagger config (%v): %v", config, err)
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

func tagger_scm_config_git(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if config == nil || !info.IsDir() {
		return
	}

	git_conf_path := filepath.Join(path, ".git/config")
	info, err := os.Stat(git_conf_path)
	if err != nil || info == nil || info.Mode() & os.ModeType != 0 {
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

func tagger_scm_config_hg(name string, config interface{}, log *logging.Logger, path string, info os.FileInfo, ctx *map[string]interface{}) (tags []string) {
	if config == nil || !info.IsDir() {
		return
	}

	hgrc_path := filepath.Join(path, ".hg/hgrc")
	info, err := os.Stat(hgrc_path)
	if err != nil || info == nil || info.Mode() & os.ModeType != 0 {
		return
	}
	hgrc, err := ini.LoadFile(hgrc_path)
	if err != nil {
		log.Warnf("Failed to parse hgrc config (%v): %v", hgrc_path, err)
		return
	}

	for _, v := range hgrc.Section("paths") {
		host := string(hg_url_pattern.ExpandString([]byte{},
			"${host}", v, hg_url_pattern.FindStringSubmatchIndex(v)))
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

	return
}


// Map of available Tagger functions
var taggers = map[string]tagger_func {
	"scm_detect_paths": tagger_scm_detect_paths,
	"lang_detect_paths": tagger_lang_detect_paths,
	"lang_detect_shebang": tagger_lang_detect_shebang,
	"scm_config_git": tagger_scm_config_git,
	"scm_config_hg": tagger_scm_config_hg,
}
var taggers_confproc = map[string]tagger_confproc {
	"scm_config_git": tagger_scm_host_confproc,
	"scm_config_hg": tagger_scm_host_confproc,
}


func init() {
	// Compile patterns for tagger_lang_detect_paths
	for re_base, tag := range lang_ext_map {
		re_base = "\\.(" + re_base +
			")(\\.(in|tpl|(src-)?bak|backup|default|example|sample|dist|\\w+-new)|_t)?$"
		lang_path_regexps = append(lang_path_regexps, path_tag_pattern{re.MustCompile(re_base), tag})
	}
	for re_base, tag := range lang_path_map {
		lang_path_regexps = append(lang_path_regexps, path_tag_pattern{re.MustCompile(re_base), tag})
	}
	for re_base, tag := range lang_interpreter_map {
		re_base = "^" + re_base + "$"
		lang_shebang_regexps = append(lang_shebang_regexps, path_tag_pattern{re.MustCompile(re_base), tag})
	}
}
