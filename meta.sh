#!/usr/bin/env bash

# 提取 makdown 文件的 metadata

meta=$(sed -n -e '0,/^$/p' $1)

file=$(echo $1|sed -E 's/^\.//'|sed -E 's/\.md$/.html/')
title=$(echo "$meta"|grep title|sed -E 's/\w+:\s+//'|sed 's/"//g')
date=$(echo "$meta"|grep date|sed -E 's/\w+:\s+//')

echo "$date,$file,$title"
