<!doctype html>
<html>
  <head>
    ${ head.tpl() }
    <title>$site_title$</title>
  </head>
  <body>
    <h1>$site_title$</h1>
    $if(site_desc)$
    <p class="site-desc">$site_desc$</p>
    $endif$
    <ol id="articles" reversed>
    $for(articles)$
      <li><a href="$it.path$">$it.title$</a> <date>$it.date$</date></li>
    $endfor$
    </ol>
    <footer>
    ${ footer.tpl() }
    </footer>
  </body>
</html>
