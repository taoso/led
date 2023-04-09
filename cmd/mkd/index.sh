#!/usr/bin/env bash

dir=$(dirname $1)

if [[ -f $dir/env ]]; then
	set -o allexport
	source $dir/env
	set +o allexport
fi

# 支持跳过子目录 index.htm
if [[ ! -z "$no_child" ]]; then
	if [[ "$dir" != "." ]]; then
		exit 0
	fi
fi

tmp=$(mktemp).yml
# index.htm 没有对应的 markdown 文件件
# 所以 title 变量为空，pandoc 会输出警告信息
echo "title: no_warn" > $tmp
echo "site_title: $site_title" >> $tmp
echo "articles:" >> $tmp

find $dir -name '*.yml' \
	! -name "draft-*.yml" ! -name "index.yml" ! -name "feed.yml" \
	-exec cat {} + | sort -r >> $tmp

# 没有 markdown 则不生成 index.htm
if [[ "$(tail -n 1 $tmp)" == "articles:" ]]; then
	rm $tmp
	exit 0
fi

meta=$dir/index.yml

diff -u $tmp $meta > /dev/null

if [ $? -eq 0 ]; then
	rm $tmp
	exit 0
fi

mv $tmp $meta

pandoc -s -p -f markdown --wrap=none \
	--template index.tpl \
	--metadata-file=$meta \
	--lua-filter $LUA_FILTER \
	-o $1 /dev/null

[ -s $1 ] || rm $1
