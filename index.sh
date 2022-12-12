#!/usr/bin/env bash

mds=$(find $1 -name '*.md' ! -name "draft-*.md")

# 没有 markdown 则不生成 index.html
if [[ -z "$mds" ]]; then
	exit 0
fi

source ./env

# 支持跳过子目录 index.html
if [[ ! -z "$no_child" ]]; then
	if [[ "$1" != "." ]]; then
		exit 0
	fi
fi

echo "articles:" > $1/index.yaml
echo $mds | tr " " "\n" | xargs -I % meta.sh % | \
	sort -r |
	awk -F, '{print "- {\"path\":\""$2"\",\"title\":\""$3"\",\"date\":\""$1"\"}"}' >> $1/index.yaml

pandoc -s -p -f markdown \
	--template index.tpl \
	--metadata-file=$1/index.yaml \
	--lua-filter $LUA_FILTER \
	-o $1/index.html /dev/null
