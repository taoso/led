#!/usr/bin/env bash

trap exit SIGINT SIGTERM

while true; do
	curl -s -u $auth "$host/+/dav-events?d=60s" | sort | uniq | \
		while read ev; do
			if [[ $ev == -* ]]; then
				# 删除 MD 清理对应生成的文件
				# -lehu.in/a/b.md -> lehu.in/a/b
				f=${ev#-}
				f=${f%.md}
				rm -f ~/sync/$f.{htm,yml}
				# 提取域名
				# -lehu.in/a/b.md -> lehu.in
				ev=${ev%%/*}
				ev=${ev#-}
			else
				# 新增或修改 MD 文件
				# +lehu.in -> lehu.in
				ev=${ev#+}
			fi
			# 重新增量构建
			build.sh $ev &
		done
	done
done
