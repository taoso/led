#!/usr/bin/env bash

trap exit SIGINT SIGTERM

while true; do
	curl -s -K ~/.curl-dav-auth "$davhost/+/dav-events?d=60s" | \
		while read ev; do
			if [[ $ev == -* ]]; then
				# 删除 MD 清理对应生成的文件
				# -lehu.in/a/b.md -> lehu.in/a/b
				f=${ev#-}
				f=${f%.md}
				rm -f ~/sync/$f.{htm,yml,*.svg}
				# 提取域名
				# lehu.in/a/b -> lehu.in
				ev=${f%%/*}
			else
				# 新增或修改 MD 文件
				# +lehu.in -> lehu.in
				ev=${ev#+}
			fi
			# 重新增量构建
			build.sh $ev &
		done
	done
