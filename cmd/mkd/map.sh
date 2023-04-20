#!/usr/bin/env bash

# 生成 sitemap 文件

source ./env

cat << EOF
<?xml version="1.0" encoding="utf-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
EOF

# - {"path":"/go/monkey.html","title":"Go语言实现猴子补丁","date":"2021-08-28"}
grep '^- ' $1/index.yml | while read line; do
	path=$(echo $line|cut -d\" -f8)
	link="$site_url$path"
	updated=`date -u +"%Y-%m-%dT%H:%M:%SZ" -r $1${path/%html/htm}`

	cat << EOF
  <url>
    <loc>${link}</loc>
    <lastmod>${updated}</lastmod>
  </url>
EOF
done

echo "</urlset>"
