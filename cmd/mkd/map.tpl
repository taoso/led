<?xml version="1.0" encoding="utf-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  $for(articles)$
  <url>
    <loc>$site_url$$it.path$</loc>
    <lastmod>$it.updated$</lastmod>
  </url>
  $endfor$
</urlset>
