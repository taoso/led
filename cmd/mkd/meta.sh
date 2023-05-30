#!/usr/bin/env bash

if [[ $(uname -s) == "Darwin" ]]; then
	updated=`date -jf "%s" "$(stat -f "%m" $1)" "+%Y-%m-%dT%H:%M:%SZ"`
else
	updated=`date -u +"%Y-%m-%dT%H:%M:%SZ" -r $1`
fi

pandoc -s -p -f markdown --wrap=none \
	--template $ROOT_DIR/meta.tpl \
	--metadata=updated:$updated \
	--lua-filter $ROOT_DIR/meta.lua \
	-o - $1
