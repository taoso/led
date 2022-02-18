#!/usr/bin/env bash

# 生成 atom 订阅文件
# 需要在 index.sh 执行之后运行

source ./env

cat << EOF
<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>$site_title</title>
  <id>$site_url/</id>
  <author>
    <name>$author_name</name>
    <email>$author_email</email>
  </author>
  <link href="$site_url"/>
  <link href="$site_url/feed.xml" rel="self"/>
  <updated>$(date -u +"%Y-%m-%dT%H:%M:%SZ")</updated>
EOF

# - {"path":"/go/monkey.html","title":"Go语言实现猴子补丁","date":"2021-08-28"}
grep '^- {"path":' $1/index.yaml | head -n 10 | while read line; do
	path=$(echo $line|cut -d\" -f4)
	link="$site_url$path"
	title=$(echo $line|cut -d\" -f8)
	md=".${path//.html/.md}"
	published=$(echo $line|cut -d\" -f12)"T00:00:00+08:00"
	updated=`date -u +"%Y-%m-%dT%H:%M:%SZ" -r $md`
	summary=`1p.sh $md`

	cat << EOF
  <entry>
    <id>$link</id>
    <link href="$link"/>
    <title>$title</title>
    <updated>$updated</updated>
    <published>$published</published>
    <summary type="html"><![CDATA[$summary]]></summary>
  </entry>
EOF
done

echo "</feed>"
