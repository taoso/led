#!/usr/bin/env bash

mds=$(find $1 -name '*.md' ! -name "draft-*.md")

# 没有 markdown 则不生成 index.html
if [[ -z "$mds" ]]; then
	exit 0
fi

# 支持跳过子目录 index.html
if [[ ! -z "$no_child" ]]; then
	if [[ "$1" != "." ]]; then
		exit 0
	fi
fi

# index.html 没有对应的 markdown 文件件
# 所以 title 变量为空，pandoc 会输出警告信息
echo "title: no_warn" > $1/index.yaml
echo "articles:" >> $1/index.yaml
echo $mds | tr " " "\n" | xargs -I % meta.sh % | \
	sort -r >> $1/index.yaml

pandoc -s -p -f markdown --wrap=none \
	--template index.tpl \
	--metadata-file=$1/index.yaml \
	--lua-filter $LUA_FILTER \
	-o $1/index.html /dev/null
