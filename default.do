# -*- mode: sh -*-

PROJECT=codetag
BIN_FILE=$PROJECT

export GOPATH=$PWD

DEPS=$(find src -type f -name '*.go')

redo-ifchange $DEPS

# Local pre/post-processing, if used with git
grep -q filter=golang .gitattributes 2>/dev/null\
	&& gpp=$(git config --get filter.golang.clean | cut -d' ' -f1)\
	|| gpp=

err=0
[[ -z "$gpp" ]] || ls -1 $DEPS | xargs -n1 $gpp curl -b

output=$(mktemp)
go build -o "$3" "${PROJECT}/cli" >"$output" 2>&1 || err=$?

if [[ -z "$gpp" ]]
then cat "$output" >&2 ||:
else
	gawk 'match($0, /^(src\/\S+\.go):([0-9]+):/, a) {
		print; system("'"$gpp"' -n show -p -c2 -n" a[2] " " a[1]);
		next} {print}' "$output" >&2 ||:
	# Doesn't highlight the relevant line:
	#  print; system("grep -nC3 . " a[1] " | grep -3 \"^" a[2] ":\"");
fi
rm -f "$output" ||:

[[ -z "$gpp" ]] || ls -1 $DEPS | xargs -n1 $gpp uncurl -b
exit $err
