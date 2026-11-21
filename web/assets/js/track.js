/* 订单查询
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var UI = window.UI;
  var input = document.getElementById('orderNo');
  var result = document.getElementById('result');
  var emptyTip = document.getElementById('emptyTip');
  var errorEl = document.getElementById('trackError');

  var ORDER = ['pending', 'processing', 'done'];

  function paintTimeline(status) {
    var reached = status === 'cancelled' ? -1 : ORDER.indexOf(status);
    document.querySelectorAll('.timeline-step').forEach(function (el, index) {
      el.classList.toggle('done', index <= reached);
    });
  }

  function show(order) {
    emptyTip.style.display = 'none';
    result.style.display = 'grid';
    document.getElementById('rNo').textContent = order.orderNo;
    document.getElementById('rStatus').innerHTML = UI.status(order.status);
    document.getElementById('rProduct').textContent = order.productTitle;
    document.getElementById('rGame').textContent = order.game || '--';
    document.getElementById('rQty').textContent = order.qty;
    document.getElementById('rAmount').textContent = '¥' + UI.money(order.amount);
    document.getElementById('rAccount').textContent = order.gameAccount || '--';
    document.getElementById('rServer').textContent = order.server || '--';
    document.getElementById('rContact').textContent = order.contactName + '（' + order.contactPhone + '）';
    document.getElementById('rCreated').textContent = UI.date(order.createdAt);
    document.getElementById('rNote').textContent = order.note || '无';
    document.getElementById('rAssignee').textContent = order.assigneeName || '待安排';
    var items = order.progress || [];
    var box = document.getElementById('rProgressBox');
    box.style.display = items.length ? 'block' : 'none';
    document.getElementById('rProgress').innerHTML = items.map(function (p) {
      return '<li><b>' + UI.date(p.at) + ' · ' + UI.escape(p.byName || '护航') + '</b><span>' + UI.escape(p.text) + '</span></li>';
    }).join('');
    paintTimeline(order.status);
    if (window.PAYUI) PAYUI.render(document.getElementById('payBox'), order, order.payment);
    if (order.status === 'cancelled') {
      UI.toast('该订单已取消，如有疑问请联系客服', 'err');
    }
  }

  async function query(no) {
    errorEl.textContent = '';
    var btn = document.getElementById('trackBtn');
    if (!no) {
      errorEl.textContent = '请输入订单号';
      return;
    }
    btn.disabled = true;
    btn.textContent = '查询中…';
    try {
      var order = await API.get('/api/orders/' + encodeURIComponent(no));
      show(order);
    } catch (err) {
      result.style.display = 'none';
      emptyTip.style.display = 'block';
      errorEl.textContent = err.message;
    } finally {
      btn.disabled = false;
      btn.textContent = '查询进度';
    }
  }

  document.getElementById('trackForm').addEventListener('submit', function (e) {
    e.preventDefault();
    query(input.value.trim().toUpperCase());
  });

  async function init() {
    try {
      var site = await API.get('/api/site');
      UI.hydrateSite(site.settings);
    } catch (err) {
      /* ignore */
    }
    var no = UI.qs('no');
    if (no) {
      input.value = no;
      query(no.toUpperCase());
    }
  }

  init();
})();
