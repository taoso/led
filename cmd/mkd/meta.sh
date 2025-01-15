#!/usr/bin/env bash

meta_file=$(mktemp)

cleanup() {
  rm -f "$meta_file"
}

trap cleanup EXIT INT TERM

title=$(basename $1 .md)
date=$(stat -c "%w" $1|cut -d' ' -f1)
updated=`date -u +"%Y-%m-%dT%H:%M:%SZ" -r $1`

echo title: $title >> $meta_file
echo date: $date >> $meta_file
echo updated: $updated >> $meta_file

pandoc -s -p -f markdown --wrap=none \
	--template $ROOT_DIR/meta.tpl \
	--metadata-file=$meta_file \
	--lua-filter $ROOT_DIR/meta.lua \
	-o - $1
