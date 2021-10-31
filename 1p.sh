#!/usr/bin/env bash

# 提取 markdown 的第一段内容，路过 metadata 部分

skip_meta=""
while IFS= read -r line; do
	if [ -n "$line" ] && [ -z "$skip_meta" ]; then
		continue
	fi
	if [ -z "$line" ] && [ -z "$skip_meta" ]; then
		skip_meta="1"
		continue
	fi
	if [ -z "$line" ] && [ -n "$skip_meta" ]; then
		break
	fi
	if [ -z "$2" ];then 
		echo $line | pandoc
	else
		echo $line
	fi
done < "$1"
