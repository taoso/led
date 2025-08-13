#!/usr/bin/env bash

# 生成 atom 订阅文件
# 需要在 index.sh 生成 index.yml 之后运行
if [[ ! -f $1/index.yml ]]; then
	exit 0
fi

updated=`date -u +"%Y-%m-%dT%H:%M:%SZ"`

if [[ -f $1/env ]]; then
	set -o allexport
	source $1/env
	set +o allexport
fi

# Feed 最多保留十条记录
# 但头两行是 tiltle 和 articles
# 所以从第三行开始
head -n 12 $1/index.yml > $1/feed.yml

pandoc -s -p -f markdown -t html --wrap=none \
	--metadata=all_updated:$updated \
	--metadata-file=$1/feed.yml \
	--template $ROOT_DIR/feed.tpl \
	--lua-filter $LUA_FILTER \
	-o $1/feed.xml /dev/null

pandoc -s -p -f markdown -t html --wrap=none \
	--metadata-file=$1/index.yml \
	--template $ROOT_DIR/map.tpl \
	--lua-filter $LUA_FILTER \
	-o $1/map.xml /dev/null
