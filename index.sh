#!/usr/bin/env bash

# 为所有文章生成 index.html 文件

diff <(ls $1/*.md|sed -E 's/\.md/.html/') <(ls $1/*.html) | \
	grep '>' | \
	grep -v 'index.html' | \
	awk '{print $2}' | \
	xargs rm -f %

source ./env

echo "site_title: $site_title" > $1/index.yaml
echo "site_url: $site_url" >> $1/index.yaml
echo "author_name: $author_name" >> $1/index.yaml
echo "author_email: $author_email" >> $1/index.yaml
echo "articles:" >> $1/index.yaml
find $1 -name '*.md' -exec meta.sh {} \; | \
	sort -r |
	awk -F, '{print "- {\"path\":\""$2"\",\"title\":\""$3"\",\"date\":\""$1"\"}"}' >> $1/index.yaml

pandoc -s -p --template index.tpl --metadata-file=$1/index.yaml -o $1/index.html /dev/null
