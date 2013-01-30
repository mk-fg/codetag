codetag
--------------------

A tool to scan "development area" (paths with code projects in them) and attach
[tmsu](http://tmsu.org/) tags to their files, according to several pre-defined
taxonomies (source language, project hosting - github/bitbucket/etc and various
other details).

End result should be tags "lang:go" and "hosted:github" for
"src/codetag/main.go" file in local clone of this repo and "lang:doc" +
"meta:readme" + "hosted:github" for this (README.md) file.

Idea is to then the next time I'll have flashback like "hey, I hacked such thing
for some public project before" to do something like `tmsu files hosted:github |
xargs grep some_code_feature` instead of rewriting it from scratch.
[tmsu](http://tmsu.org/)-provided fuse-fs might make it even easier.

Under heavy development, not ready yet.
