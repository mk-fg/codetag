codetag
--------------------

A tool to scan "development area" (paths with code projects in them) and attach
[tmsu](http://tmsu.org/) tags to their files, according to several pre-defined
taxonomies (e.g. source language, project hosting - github/bitbucket/etc and
various other details).

End result should be tags "lang:go" + "scm:git" + "host:github" for
"src/codetag/main.go" file in local clone of this repo and "lang:md" +
"meta:readme" + "scm:git" + "hosted:github" for this (README.md) file.

One use of such tagging is to avoid grep-scan of a huge body of source code
(incl. forks, sdks, temp build paths) and - especially for simple grep - huge
body of other data, using something like `tmsu files lang:py hosted:github |
xargs grep some_code_feature` (s/grep/ack/ or pss if appropriate) instead to
find something you wrote a while ago (and can't recall where exactly).

[tmsu](http://tmsu.org/)-provided fuse-fs might make it even easier.

There are few code-indexing systems around in free software world, like
[ctags](http://ctags.sourceforge.net/), which seem to be geared towards indexing
source code contents, picking structural details from these, not towards just
indexing code metadata.


Installation
--------------------

First, install (or build) tmsu [from here](http://tmsu.org/) (single binary, see
[README](https://bitbucket.org/oniony/tmsu/) there on how to build it from
source).

Same as tmsu, this tool itself is written in [Go](http://golang.org/), which
doesn't require any runtime to be around for it's statically-linked binaries to
work, so 32-bit linux binary (should work on amd64 arch as well) can be
[downloaded here](http://fraggod.net/static/code/codetag) (usual warning about
downloading binaries via http applies).

To build the thing from source:

 * [Install Go](http://www.golang.org/).

 * Fetch the go package deps:

		go get github.com/mk-fg/go-logging
		go get github.com/kylelemons/go-gypsy/yaml

 * Get the code: `git clone https://github.com/mk-fg/codetag`

 * Build the code:

		cd codetag
		make

 * Binary will be in the "bin/codetag" path, and can be installed via usual
   means e.g. `sudo install -m755 bin/codetag /usr/local/bin`.


Usage
--------------------

Tool uses configuration file to keep track of all the settings, which is
searched (by default, if "--config" option is not used) in the following paths
(in that order):

	<binary path>.yaml
	~/.codetag.yaml
	/etc/codetag.yaml

So create configuration file in e.g. "~/.codetag.yaml" by copying
[codetag.yaml.dist](https://github.com/mk-fg/codetag/blob/master/codetag.yaml.dist)
from the repository.

Most important section is the first one - "paths".
Path(s) there should be set to some "~/projects" directory(-ies) used for the
code that should be taggged:

	paths: ~/projects

Or, for several paths:

	paths:
	  - ~/projects
	  - ~/src
	  - ~/work

"taggers" section there will define tag namespaces and how tags in these should
be generated.
For example, "lang: lang_detect_paths" there will use "lang_detect_paths" plugin
to set tags like "lang:py", based on path/filename patterns.

"logging" and "filtering" sections might be useful to keep track of errors and
control noise (e.g. if used from cron, set log level to WARNING there, refer to
[go-logging](https://github.com/mk-fg/go-logging) docs for more details) or
exclude large and irrelevant paths from tagging (e.g. insides of ".git"
directories, as done there by default).

Refer to annotations in the [example configuration
file](https://github.com/mk-fg/codetag/blob/master/codetag.yaml.dist) for
reference on all the options there.

When done with config, just run the tool.
It will run "tmsu" binary to attach detected tags to files within the scanned dirs.

Then, just use tmsu ([examples/docs](http://tmsu.org/)) as usual to get the list
of files by tags, e.g.:

	codetag % tmsu files scm:git lang:go
	src/codetag/log_setup/log_setup.go
	src/codetag/main.go
	src/codetag/taggers/taggers.go

Or use tmsu's awesome mount command:

	codetag % mkdir mp
	codetag % tmsu mount mp
	codetag % ls -l mp/tags/scm:git/lang:go
	 drwxr-xr-x 0 fraggod fraggod    10 Feb  4 14:31 host:github
	 drwxr-xr-x 0 fraggod fraggod     6 Feb  4 14:31 host:local
	 lrwxr-xr-x 1 fraggod fraggod  2959 Feb  2 13:14 log_setup.12.go
	 lrwxr-xr-x 1 fraggod fraggod 10395 Feb  4 12:50 main.14.go
	 lrwxr-xr-x 1 fraggod fraggod  7833 Feb  4 11:05 taggers.16.go

And feel free to hack additional taggers, it's rather trivial - see
[taggers.go](https://github.com/mk-fg/codetag/blob/master/src/codetag/taggers/taggers.go).

Hope that helps, have fun!
