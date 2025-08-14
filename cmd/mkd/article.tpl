<!doctype html>
<html>
  <head>
    ${ head.tpl() }
    <title>$title$</title>
    <meta name="description" content="$description$">
  </head>
  <body>
  <article>
  <h1>$title$</h1>
  <div class="meta">
  $if(author_name)$
  <a rel="author" href="$author_url$">$author_name$</a>
  $endif$
  <date>$date$</date>
  <span>⏳$read_time$分钟($runes$千字)</span>
  </div>
  $if(toc)$
  <nav id="TOC" role="doc-toc">
  $table-of-contents$
  </nav>
  $endif$
  $body$
  </article>
  <footer>
  ${ footer.tpl() }
  </footer>
  </body>
</html>
