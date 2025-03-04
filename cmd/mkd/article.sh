#!/usr/bin/env bash

meta_file=$(mktemp)

cleanup() {
  rm -f "$meta_file"
}

trap cleanup EXIT INT TERM

echo title: $(basename $1 .md) >> $meta_file
echo date: $(stat -c "%w" $1|cut -d' ' -f1) >> $meta_file

pandoc -s -p --wrap=none \
	--toc \
	--mathml \
	--template article.tpl \
	--highlight-style=pygments \
	--lua-filter $LUA_FILTER \
	--metadata-file=$meta_file \
	--from markdown+east_asian_line_breaks \
	$1 -o $2
