PROJECT=codetag
BIN_FILE=$PROJECT

export GOPATH=$PWD

DEPS=$(find src -type f -name '*.go')

redo-ifchange $DEPS

err=0
ls -1 $DEPS | xargs -n1 golang_filter curl
go build -o "$3" "${PROJECT}" >&2 || err=$?
ls -1 $DEPS | xargs -n1 golang_filter uncurl

exit $err
