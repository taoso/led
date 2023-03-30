#!/usr/bin/env bash

# 生成 atom 订阅文件
# 需要在 index.sh 生成 index.yml 之后运行

updated=`date -u +"%Y-%m-%dT%H:%M:%SZ"`

# Feed 最多保留十条记录
# 但头两行是 tiltle 和 articles
# 所以从第三行开始
head -n 12 $1/index.yml > $1/feed.yml

pandoc -s -p -f markdown --wrap=none \
	--metadata=updated:$updated \
	--metadata-file=$1/feed.yml \
	--template $ROOT_DIR/feed.tpl \
	--lua-filter $LUA_FILTER \
	-o - /dev/null
