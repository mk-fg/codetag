# -*- mode: sh -*-

PROJECT=codetag

DEPS=$(find -type f -name '*.go')

redo-ifchange $DEPS

mkdir -p bin

# Local pre/post-processing, if used with git
grep -q filter=golang .gitattributes 2>/dev/null\
	&& gpp=$(git config --get filter.golang.clean | cut -d' ' -f1)\
	|| gpp=

err=0
[[ -z "$gpp" ]] || ls -1 $DEPS | xargs -n1 $gpp curl -b

output=$(mktemp)
go build -o "$3" "${PROJECT}" >"$output" 2>&1 || err=$?

if [[ -z "$gpp" ]]
then cat "$output" >&2 ||:
else
	gawk 'match($0, /^((..)\/\S+\.go):([0-9]+):/, a) {
		print; system("'"$gpp"' -n show -p -c2 -n" a[3] " " a[1]);
		next} {print}' "$output" >&2 ||:
	# Doesn't highlight the relevant line:
	#  print; system("grep -nC3 . " a[1] " | grep -3 \"^" a[3] ":\"");
fi
rm -f "$output" ||:

[[ -z "$gpp" ]] || ls -1 $DEPS | xargs -n1 $gpp uncurl -b

[[ $err -ne 0 ]] || {
	# Upload to destinations, listed in .gitattributes, if any
	gawk 'match($2, /upload=(.*)/, a) {print $1, a[1]}' .gitattributes 2>/dev/null |
	while read bin upload; do
		[[ "$bin" = "$1" || "$bin" = "/$1" ]] || continue
		dst="$upload/$(basename "$1")"
		dst_tmp=$(mktemp)
		cp "$3" "$dst_tmp" &&\
			strip "$dst_tmp" &&\
			chmod 755 "$dst_tmp" &&\
			mv "$dst_tmp" "$dst" ||\
			{ rm -f "$dst_tmp"; echo "Failed to upload $bin to $dst"; }
	done >&2
	# Symlink to destinations, listed in .gitattributes, if any
	gawk 'match($2, /link=(.*)/, a) {print $1, a[1]}' .gitattributes 2>/dev/null |
	while read bin link_dst; do
		[[ "$bin" = "$1" || "$bin" = "/$1" ]] || continue
		dst="${link_dst}/$(basename "$1")"
		ln -sf "$(realpath "$1")" "$dst"
	done
}

exit $err
