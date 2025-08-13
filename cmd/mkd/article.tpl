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
  </div>
  $if(toc)$
  <nav id="TOC" role="doc-toc">
  $table-of-contents$
  </nav>
  $endif$
  $body$
  </article>
  $if(author_email)$
  <form class="reply" onsubmit="event.preventDefault(); reply()">
    <textarea name="content"
      $if(lang_en)$
      placeholder="Welcome to leave message for discussion.&#10;Message content and contact information are only visible to the author.&#10;Please leave your usual email address to recive author's reply."
      $else$
      placeholder="欢迎留言讨论。&#10;留言内容和联系信息仅作者可见。&#10;请留下常用邮箱以接收作者回复。"
      $endif$
      required></textarea>
    $if(lang_en)$
    <input class="author" required name="email" type="email" placeholder="Email (required)">
    <input class="author" name="name" type="text" placeholder="Name (optional)">
    <input type="submit" value="Submit">
    $else$
    <input class="author" required name="email" type="email" placeholder="邮箱（必填）">
    <input class="author" name="name" type="text" placeholder="名字（选填）">
    <input type="submit" value="提交">
    $endif$
  </form>
  <script>
  function reply() {
    var f = event.target;
    f.querySelector('input[type="submit"]').disabled = true;
    var data = new FormData(f);
    data.append("subject", document.title);
    fetch('/+/mail', {
      method: 'POST',
      body: new URLSearchParams(data),
    })
    .then(resp => alert("留言提交成功"))
    .catch(error => alert("接口报错："+error))
    .finally(() => {
      f.querySelector('textarea').value = '';
      f.querySelector('input[type="submit"]').disabled = false;
    });
  }
  </script>
  $endif$
  <footer>
  ${ footer.tpl() }
  </footer>
  </body>
</html>
