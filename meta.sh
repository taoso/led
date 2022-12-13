#!/usr/bin/env bash

pandoc -s -p -f markdown \
	--template meta.tpl \
	--lua-filter $ROOT_DIR/meta.lua \
	-o - $1
