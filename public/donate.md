---
title: 支持作者
date: 2025-03-16
ga_id: ""
author_email: ""
index: false
toc: false
---

如果我写的内容让你的生活变好了一点点，欢迎请我喝一杯咖啡☕️🙏

捐助通过 Stripe 收款。付款成功后回你的大名会出现在功德榜。

<!-- CURRENCY 要和站点 env 里的 stripe_currency 一致；
     金额档位就是下面这几个 radio，想改直接改 value 和标签 -->

<p id="done" hidden><strong>捐助已收到，非常谢谢你！</strong></p>

<style>
#amounts {
  label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
  }
}

/* 功德榜：无边框卡片行，靠发丝分隔线和留白区分层次 */
#donors {
  list-style: none;
  margin: 0;
  padding: 0;

  li {
    display: flex;
    gap: 12px;
    padding: 14px 0;
    border-top: 1px solid var(--border, #e5e7eb);
  }

  li:first-child {
    border-top: 0;
    padding-top: 4px;
  }

  .avatar {
    flex: 0 0 36px;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--background-alt, #f1f5f9);
    color: var(--text-muted, #64748b);
    font-size: 15px;
    font-weight: 600;
    user-select: none;
  }

  .item {
    flex: 1 1 auto;
    min-width: 0;
  }

  .row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
  }

  .name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .amount {
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .msg {
    margin: 4px 0 0;
    color: var(--text-muted, #64748b);
    overflow-wrap: anywhere;
  }

  time {
    display: block;
    margin-top: 4px;
    font-size: 13px;
    color: var(--text-muted, #64748b);
    font-variant-numeric: tabular-nums;
  }

  .empty {
    display: block;
    padding: 16px 0;
    color: var(--text-muted, #64748b);
  }
}
</style>
<form id="donate">
  <p id="amounts">
    <label><input type="radio" name="amount" value="1" checked> <span>1</span></label>
    <label><input type="radio" name="amount" value="5"> <span>5</span></label>
    <label><input type="radio" name="amount" value="10"> <span>10</span></label>
    <label><input type="radio" name="amount" value="20"> <span>20</span></label>
    <label><input type="radio" name="amount" value="50"> <span>50</span></label>
  </p>
  <p><input id="donor" type="text" maxlength="60" placeholder="称呼（选填，公开显示）"></p>
  <p><textarea id="message" maxlength="500" rows="2" placeholder="留言（选填，公开显示）"></textarea></p>
  <p><input id="submit" type="submit" value="捐助"></p>
  <p id="hint"></p>
</form>

<noscript>
<p>本页面需要开启 JavaScript 才能运行完整功能！</p>
</noscript>

## 功德榜

<ol id="donors"></ol>

<script>
var CURRENCY = 'usd';

var form = document.getElementById('donate');
var hint = document.getElementById('hint');
var submit = document.getElementById('submit');
var donors = document.getElementById('donors');
var radios = form.querySelectorAll('input[name="amount"]');

function money(n) {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: CURRENCY.toUpperCase(),
    }).format(n);
  } catch (e) {
    return n + ' ' + CURRENCY.toUpperCase();
  }
}

// 把 radio 旁边的数字换成带币种符号的形式，没 js 时显示原始数字
for (var i = 0; i < radios.length; i++) {
  var label = radios[i].parentNode;
  label.querySelector('span').textContent = money(Number(radios[i].value));
}

function currentAmount() {
  var checked = form.querySelector('input[name="amount"]:checked');
  return checked ? Number(checked.value) : 0;
}

// 金额已经由服务端按币种格式化，这里只做纯文本渲染
function renderDonations() {
  return fetch('/+/stripe-donations')
    .then(function (resp) { return resp.ok ? resp.json() : []; })
    .then(function (list) {
      donors.textContent = '';

      if (!list.length) {
        var none = document.createElement('li');
        none.className = 'empty';
        none.textContent = '还没有人捐助，来做第一个吧。';
        donors.appendChild(none);
        return;
      }

      list.forEach(function (d) {
        var name = d.donor || '匿名';

        var li = document.createElement('li');

        var avatar = document.createElement('div');
        avatar.className = 'avatar';
        avatar.textContent = Array.from(name)[0] || '?';   // 取首字做头像

        var item = document.createElement('div');
        item.className = 'item';

        var row = document.createElement('div');
        row.className = 'row';

        var who = document.createElement('span');
        who.className = 'name';
        who.textContent = name;

        var amount = document.createElement('span');
        amount.className = 'amount';
        amount.textContent = d.amount;

        row.appendChild(who);
        row.appendChild(amount);
        item.appendChild(row);

        if (d.message) {
          var msg = document.createElement('p');
          msg.className = 'msg';
          msg.textContent = d.message;
          item.appendChild(msg);
        }

        var when = document.createElement('time');
        when.dateTime = d.created;
        when.textContent = d.created;
        item.appendChild(when);

        li.appendChild(avatar);
        li.appendChild(item);
        donors.appendChild(li);
      });
    });
}

form.onsubmit = function (event) {
  event.preventDefault();

  var amount = currentAmount();
  if (!(amount > 0)) {
    hint.textContent = '请先选择金额';
    return;
  }

  submit.disabled = true;
  hint.textContent = '';

  fetch('/+/stripe-donate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      amount: amount,
      donor: document.getElementById('donor').value,
      message: document.getElementById('message').value,
    }),
  })
    .then(function (resp) {
      if (!resp.ok) {
        return resp.text().then(function (text) { throw new Error(text); });
      }
      return resp.json();
    })
    .then(function (data) { window.location.href = data.url; })
    .catch(function (err) { hint.textContent = '无法发起支付：' + err.message; })
    .finally(function () { submit.disabled = false; });
};

renderDonations();

// 从收银台返回时，SuccessURL 上带着 session_id
if (new URLSearchParams(window.location.search).get('session_id')) {
  document.getElementById('done').hidden = false;

  // 去掉 query，免得刷新页面又看到一次
  if (window.history && history.replaceState) {
    history.replaceState(null, '', window.location.pathname);
  }

  // webhook 可能比浏览器跳转晚一步，稍后再拉一次名单
  setTimeout(renderDonations, 3000);
}
</script>
