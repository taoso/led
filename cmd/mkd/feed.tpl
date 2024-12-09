<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>$site_title$</title>
  <id>$site_url$/</id>
  <author>
    <name>$author_name$</name>
    <email>$author_email$</email>
  </author>
  <link href="$site_url$"/>
  <link href="$site_url$/feed.xml" rel="self"/>
  <updated>$all_updated$</updated>
  $for(articles)$
  <entry>
    <id>$site_url$$it.path$</id>
    <link href="$site_url$$it.path$"/>
    <title>$it.title$</title>
    <updated>$it.updated$</updated>
    <published>$it.date$T00:00:00+08:00</published>
    <summary type="html"><![CDATA[$it.desc$]]></summary>
  </entry>
  $endfor$
</feed>
