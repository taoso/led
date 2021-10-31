#!/usr/bin/env bash

# 生成 sitemap 文件

cat << EOF
<?xml version="1.0" encoding="utf-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
EOF

for md in $(find $1 -name '*.md'); do
	path=${md//.md/.html}
	link="https://taoshu.in${path#.}"
	updated=`date -u +"%Y-%m-%dT%H:%M:%SZ" -r $path`

	cat << EOF
  <url>
    <loc>${link}</loc>
    <lastmod>${updated}</lastmod>
  </url>
EOF
done

echo "</urlset>"
