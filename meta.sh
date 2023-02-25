#!/usr/bin/env bash

updated=`date -u +"%Y-%m-%dT%H:%M:%SZ" -r $1`

pandoc -s -p -f markdown --wrap=none \
	--template $ROOT_DIR/meta.tpl \
	--metadata=updated:$updated \
	--lua-filter $ROOT_DIR/meta.lua \
	-o - $1
