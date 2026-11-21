/* 客服工作台
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var UI = window.UI;
  var state = { orders: [], keyword: '', status: 'all', pay: 'all', me: null };

  function statusText(status) { return UI.statusText(status); }

  function kpis() {
    var counts = { pending: 0, review: 0, processing: 0, doneToday: 0 };
    var today = new Date().toISOString().slice(0, 10);
    state.orders.forEach(function (o) {
      if (o.status === 'pending') counts.pending++;
      if (o.status === 'processing') counts.processing++;
      if (o.payStatus === 'review') counts.review++;
      if (o.status === 'done' && String(o.updatedAt || '').slice(0, 10) === today) counts.doneToday++;
    });
    var items = [
      ['待处理', counts.pending, '含未付款，先核对到账'],
      ['待确认付款', counts.review, '核对截图后确认到账'],
      ['护航中', counts.processing, '盯一下进度'],
      ['今日完成', counts.doneToday, '记得回访']
    ];
    document.getElementById('kpis').innerHTML = items
      .map(function (it) {
        return '<div class="console-card"><span>' + it[0] + '</span><b>' + it[1] + '</b><i>' + it[2] + '</i></div>';
      })
      .join('');
  }

  function filtered() {
    var q = state.keyword.toLowerCase();
    return state.orders.filter(function (o) {
      if (state.status !== 'all' && o.status !== state.status) return false;
      if (state.pay !== 'all' && (o.payStatus || 'unpaid') !== state.pay) return false;
      if (!q) return true;
      return [o.orderNo, o.contactName, o.contactPhone, o.gameAccount, o.productTitle, o.assigneeName]
        .join(' ').toLowerCase().indexOf(q) >= 0;
    });
  }

  function acts(o) {
    var out = '<button class="btn btn-ghost btn-xs" data-act="view" data-id="' + o.id + '">详情</button>';
    if (o.status === 'pending' && (o.payStatus || 'unpaid') === 'paid') out += '<button class="btn btn-primary btn-xs" data-act="processing" data-id="' + o.id + '">接单</button>';
    if (o.status === 'processing') out += '<button class="btn btn-ghost btn-xs" data-act="done" data-id="' + o.id + '">完成</button>';
    if ((o.payStatus || 'unpaid') !== 'paid') out += '<button class="btn btn-ghost btn-xs" data-act="pay" data-id="' + o.id + '">确认到账</button>';
    if (o.status !== 'done' && o.status !== 'cancelled') out += '<button class="btn btn-ghost btn-xs" data-act="note" data-id="' + o.id + '">备注</button>';
    return out;
  }

  function cards(items) {
    var box = document.getElementById('orderCards');
    box.innerHTML = items.map(function (o) {
      return '<article class="task-card">' +
        '<div class="task-head"><span class="mono">' + UI.escape(o.orderNo) + '</span>' +
        '<span>' + UI.payStatus(o.payStatus || 'unpaid') + ' ' + UI.status(o.status) + '</span></div>' +
        '<h4>' + UI.escape(o.productTitle) + '</h4>' +
        '<div class="task-meta"><span>' + UI.escape(o.contactName) + ' ' + UI.escape(o.contactPhone) + '</span>' +
        '<span class="money">¥' + UI.money(o.amount) + '</span></div>' +
        '<div class="task-sub">' + UI.escape(o.game || '') + ' · ' + UI.escape(o.server || '') +
        ' · 打手 ' + (o.assigneeName ? UI.escape(o.assigneeName) : '未派') + '</div>' +
        '<div class="row-actions">' + acts(o) + '</div></article>';
    }).join('');
  }

  function rows() {
    var items = filtered();
    var tbody = document.getElementById('rows');
    cards(items);
    if (!items.length) {
      tbody.innerHTML = '<tr><td colspan="8" class="console-empty">没有符合条件的订单</td></tr>';
      return;
    }
    tbody.innerHTML = items
      .map(function (o) {
        var actions = acts(o);
        return (
          '<tr>' +
          '<td class="mono">' + UI.escape(o.orderNo) + '</td>' +
          '<td>' + UI.escape(o.productTitle) + '<div class="sub">' + UI.escape(o.game || '') + ' · ' + UI.escape(o.server || '') + '</div></td>' +
          '<td>' + UI.escape(o.contactName) + '<div class="sub">' + UI.escape(o.contactPhone) + '</div></td>' +
          '<td class="money">¥' + UI.money(o.amount) + '</td>' +
          '<td>' + UI.payStatus(o.payStatus || 'unpaid') + '</td>' +
          '<td>' + UI.status(o.status) + '</td>' +
          '<td>' + (o.assigneeName ? UI.escape(o.assigneeName) : '<span class="sub">未派单</span>') + '</td>' +
          '<td><div class="row-actions">' + actions + '</div></td>' +
          '</tr>'
        );
      })
      .join('');
  }

  async function load() {
    try {
      state.orders = (await API.get('/api/admin/orders')).items;
      kpis();
      rows();
    } catch (err) {
      UI.toast(err.message, 'err');
    }
  }

  function find(id) {
    return state.orders.find(function (o) { return o.id === id; });
  }

  function detail(order) {
    var progress = order.progress || [];
    document.getElementById('detailBody').innerHTML =
      '<div class="detail-grid">' +
      '<div><span>订单号</span><b>' + UI.escape(order.orderNo) + '</b></div>' +
      '<div><span>服务</span><b>' + UI.escape(order.productTitle) + '</b></div>' +
      '<div><span>客户</span><b>' + UI.escape(order.contactName) + '　' + UI.escape(order.contactPhone) + '</b></div>' +
      '<div><span>游戏账号</span><b>' + UI.escape(order.gameAccount) + '</b></div>' +
      '<div><span>区服</span><b>' + UI.escape(order.server || '—') + '</b></div>' +
      '<div><span>金额 / 付款</span><b>¥' + UI.money(order.amount) + '　' + UI.payStatusText(order.payStatus || 'unpaid') + '</b></div>' +
      '</div>' +
      '<div class="detail-block"><span>客户备注</span><p>' + UI.escape(order.note || '无') + '</p></div>' +
      '<div class="detail-block"><span>内部备注</span><p>' + UI.escape(order.supportNote || '无') + '</p></div>' +
      '<div class="detail-block"><span>进度</span>' +
      (progress.length
        ? '<ul class="progress-list">' + progress.map(function (p) {
            return '<li><b>' + UI.date(p.at) + ' · ' + UI.escape(p.byName || '') + '</b><span>' + UI.escape(p.text) + '</span></li>';
          }).join('') + '</ul>'
        : '<p class="sub">还没有进度记录</p>') +
      '</div>';
    document.getElementById('detailMask').classList.add('show');
  }

  async function act(kind, order) {
    try {
      if (kind === 'view') return detail(order);
      if (kind === 'processing' || kind === 'done') {
        await API.put('/api/admin/orders/' + encodeURIComponent(order.id), { status: kind });
        UI.toast(kind === 'processing' ? '已接单，记得拉群对齐时间' : '已标记完成', 'ok');
      }
      if (kind === 'pay') {
        await API.put('/api/admin/orders/' + encodeURIComponent(order.id) + '/payment', { action: 'confirm' });
        UI.toast('已确认到账', 'ok');
      }
      if (kind === 'note') {
        var note = prompt('内部备注（客户看不到）', order.supportNote || '');
        if (note === null) return;
        await API.put('/api/admin/orders/' + encodeURIComponent(order.id), { supportNote: note });
        UI.toast('备注已保存', 'ok');
      }
      load();
    } catch (err) {
      UI.toast(err.message, 'err');
    }
  }

  function bind() {
    ['rows', 'orderCards'].forEach(function (id) {
      document.getElementById(id).addEventListener('click', function (e) {
        var btn = e.target.closest('button[data-act]');
        if (!btn) return;
        var order = find(btn.getAttribute('data-id'));
        if (order) act(btn.getAttribute('data-act'), order);
      });
    });
    document.getElementById('filterStatus').addEventListener('change', function () {
      state.status = this.value; rows();
    });
    document.getElementById('filterPay').addEventListener('change', function () {
      state.pay = this.value; rows();
    });
    var timer = null;
    document.getElementById('search').addEventListener('input', function () {
      var value = this.value.trim();
      clearTimeout(timer);
      timer = setTimeout(function () { state.keyword = value; rows(); }, 220);
    });
    document.getElementById('detailClose').addEventListener('click', function () {
      document.getElementById('detailMask').classList.remove('show');
    });
    document.getElementById('detailMask').addEventListener('click', function (e) {
      if (e.target === this) this.classList.remove('show');
    });
  }

  async function init() {
    bind();
    var ok = await STATION.guard(['owner', 'manager', 'support'], function (me) {
      state.me = me;
      load();
    });
    if (!ok) return;
  }

  init();
})();
