all:
	@if which redo >/dev/null 2>&1; then redo; else ./do; fi
