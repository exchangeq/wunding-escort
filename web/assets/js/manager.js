/* 管事端
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';
  var UI = window.UI;
  var state = { orders: [], products: [], boosters: [], users: [], settings: null, status: 'all', q: '' };

  function cards(t) {
    var items = [
      ['订单总数', t.orders, '单'],
      ['待处理', t.pending, '含未付款'],
      ['护航中', t.processing, '单'],
      ['待确认付款', t.payReview || 0, '单'],
      ['未派单', t.unassigned || 0, '单'],
      ['在岗打手', t.boosters || 0, '人'],
      ['已完成收入', '¥' + UI.money(t.revenue), '']
    ];
    document.getElementById('kpis').innerHTML = items.map(function (it) {
      return '<div class="console-card"><span>' + it[0] + '</span><b>' + UI.escape(it[1]) + '</b><i>' + it[2] + '</i></div>';
    }).join('');
  }

  async function loadOverview() {
    try {
      var data = await API.get('/api/admin/overview');
      cards(data.totals);
      var max = Math.max.apply(null, data.series.map(function (s) { return s.count; }).concat([1]));
      document.getElementById('chart').innerHTML = data.series.map(function (s) {
        var h = Math.max(6, Math.round((s.count / max) * 110));
        return '<div class="chart-col"><strong>' + s.count + '</strong><div class="chart-bar" style="height:' + h + 'px"></div><span>' + s.date + '</span></div>';
      }).join('');
      loadQueue();
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function loadQueue() {
    var box = document.getElementById('overviewQueue');
    try {
      var data = await API.get('/api/admin/orders?status=pending');
      var items = data.items.filter(function (o) { return o.payStatus === 'paid' && !o.assigneeId; }).slice(0, 5);
      if (!items.length) { box.innerHTML = '<div class="console-empty">没有待派单，队列很干净</div>'; return; }
      box.innerHTML = items.map(function (o) {
        return '<div class="queue-row"><div><b>' + UI.escape(o.productTitle) + '</b><span class="sub">' + UI.escape(o.orderNo) + ' · ' + UI.escape(o.contactName) + '</span></div>' +
          '<span class="money">¥' + UI.money(o.amount) + '</span>' +
          '<button class="btn btn-primary btn-xs" data-goto="orders">去派单</button></div>';
      }).join('');
    } catch (err) { box.innerHTML = '<div class="console-empty">' + UI.escape(err.message) + '</div>'; }
  }

  async function loadOrders() {
    var rows = document.getElementById('oRows');
    rows.innerHTML = '<tr><td colspan="9" class="console-empty">加载中…</td></tr>';
    try {
      var data = await API.get('/api/admin/orders' + UI.params({ status: state.status, q: state.q }));
      state.orders = data.items;
      if (!data.items.length) {
        rows.innerHTML = '<tr><td colspan="9" class="console-empty">没有订单</td></tr>';
        return;
      }
      var cards = [];
      rows.innerHTML = data.items.map(function (o) {
        var options = '<option value="">未分配</option>' + state.boosters.map(function (b) {
          return '<option value="' + b.id + '"' + (o.assigneeId === b.id ? ' selected' : '') + '>' + UI.escape(b.name) + '</option>';
        }).join('');
        var acts = '';
        if (o.status === 'pending' && (o.payStatus || 'unpaid') === 'paid') acts += '<button class="btn btn-primary btn-xs" data-oact="processing" data-id="' + o.id + '">接单</button>';
        if (o.status === 'processing') acts += '<button class="btn btn-ghost btn-xs" data-oact="done" data-id="' + o.id + '">完成</button>';
        if ((o.payStatus || 'unpaid') !== 'paid') acts += '<button class="btn btn-ghost btn-xs" data-oact="pay" data-id="' + o.id + '">确认到账</button>';
        cards.push('<article class="task-card">' +
          '<div class="task-head"><span class="mono">' + UI.escape(o.orderNo) + '</span><span>' + UI.payStatus(o.payStatus || 'unpaid') + ' ' + UI.status(o.status) + '</span></div>' +
          '<h4>' + UI.escape(o.productTitle) + '</h4>' +
          '<div class="task-meta"><span>' + UI.escape(o.contactName) + ' ' + UI.escape(o.contactPhone) + '</span><span class="money">¥' + UI.money(o.amount) + '</span></div>' +
          '<div class="task-sub">' + UI.escape(o.game || '') + ' · 打手 ' + (o.assigneeName ? UI.escape(o.assigneeName) : '未派') + '</div>' +
          '<select class="tiny-select" data-assign="' + o.id + '">' + options + '</select>' +
          '<div class="row-actions">' + acts + '</div></article>');
        return '<tr>' +
          '<td class="mono">' + UI.escape(o.orderNo) + '</td>' +
          '<td>' + UI.escape(o.productTitle) + '<div class="sub">' + UI.escape(o.game || '') + '</div></td>' +
          '<td>' + UI.escape(o.contactName) + '<div class="sub">' + UI.escape(o.contactPhone) + '</div></td>' +
          '<td class="money">¥' + UI.money(o.amount) + '</td>' +
          '<td>' + UI.payStatus(o.payStatus || 'unpaid') + '</td>' +
          '<td>' + UI.status(o.status) + '</td>' +
          '<td>' + (o.assigneeName ? UI.escape(o.assigneeName) : '<span class="sub">未派</span>') + '</td>' +
          '<td><select class="tiny-select" data-assign="' + o.id + '">' + options + '</select></td>' +
          '<td><div class="row-actions">' + acts + '</div></td></tr>';
      }).join('');
      document.getElementById('orderCards').innerHTML = cards.join('');
    } catch (err) {
      rows.innerHTML = '<tr><td colspan="9" class="console-empty">' + UI.escape(err.message) + '</td></tr>';
      document.getElementById('orderCards').innerHTML = '<div class="console-empty">' + UI.escape(err.message) + '</div>';
    }
  }

  async function loadBoosters() {
    try {
      var data = await API.get('/api/admin/users');
      state.users = data.items;
      state.boosters = data.items.filter(function (u) { return u.role === 'booster' && u.status !== 'off'; });
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function loadProducts() {
    try {
      state.products = (await API.get('/api/admin/products')).items;
      document.getElementById('pRows').innerHTML = state.products.map(function (p) {
        return '<tr>' +
          '<td>' + UI.escape(p.title) + '<div class="sub">' + UI.escape(p.category) + '</div></td>' +
          '<td>' + UI.escape(p.game) + '</td>' +
          '<td class="money">¥' + UI.money(p.price) + '<span class="sub">/' + UI.escape(p.unit || '') + '</span></td>' +
          '<td>' + (p.stock == null ? '充足' : p.stock) + '</td>' +
          '<td>' + p.sales + '</td>' +
          '<td>' + (p.status === 'off' ? '<span class="status-pill status-cancelled">已下架</span>' : '<span class="status-pill status-done">在售</span>') + '</td>' +
          '<td><div class="row-actions"><button class="btn btn-ghost btn-xs" data-pact="price" data-id="' + p.id + '">改价</button>' +
          '<button class="btn btn-ghost btn-xs" data-pact="' + (p.status === 'off' ? 'on' : 'off') + '" data-id="' + p.id + '">' + (p.status === 'off' ? '上架' : '下架') + '</button></div></td></tr>';
      }).join('');
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function loadUsers() {
    try {
      var data = await API.get('/api/admin/users');
      state.users = data.items;
      state.boosters = data.items.filter(function (u) { return u.role === 'booster' && u.status !== 'off'; });
      document.getElementById('uRows').innerHTML = data.items.map(function (u) {
        return '<tr>' +
          '<td>' + UI.escape(u.name) + '</td><td class="mono">' + UI.escape(u.username) + '</td>' +
          '<td>' + STATION.roleName(u.role) + '</td>' +
          '<td>' + (u.role === 'booster' ? u.share + '%' : '—') + '</td>' +
          '<td>' + (u.status === 'off' ? '<span class="status-pill status-cancelled">停用</span>' : '<span class="status-pill status-done">在岗</span>') + '</td>' +
          '<td class="sub">' + UI.date(u.lastLoginAt) + '</td>' +
          '<td><div class="row-actions">' +
          '<button class="btn btn-ghost btn-xs" data-uact="' + (u.status === 'off' ? 'on' : 'off') + '" data-id="' + u.id + '">' + (u.status === 'off' ? '启用' : '停用') + '</button>' +
          '<button class="btn btn-ghost btn-xs" data-uact="pass" data-id="' + u.id + '">重置密码</button>' +
          '</div></td></tr>';
      }).join('');
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function loadSettings() {
    var s = await API.get('/api/admin/settings');
    state.settings = s;
    ['siteName', 'company', 'notice', 'orderTips'].forEach(function (k) {
      var el = document.getElementById('s-' + k);
      if (el) el.value = s[k] || '';
    });
    document.getElementById('s-kefu-qq').value = (s.kefu && s.kefu.qq) || '';
    document.getElementById('s-kefu-wechat').value = (s.kefu && s.kefu.wechat) || '';
  }

  async function loadLogs() {
    var data = await API.get('/api/admin/logs');
    document.getElementById('logList').innerHTML = data.items.map(function (l) {
      return '<li><span>' + UI.date(l.at) + '</span><b>' + UI.escape(l.action) + '</b></li>';
    }).join('');
  }

  function bind() {
    document.getElementById('consoleNav').addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-view]');
      if (!btn) return;
      var view = btn.getAttribute('data-view');
      if (view === 'overview') loadOverview();
      if (view === 'orders') { loadBoosters().then(loadOrders); }
      if (view === 'products') loadProducts();
      if (view === 'staff') loadUsers();
      if (view === 'settings') loadSettings();
      if (view === 'logs') loadLogs();
    });

    document.getElementById('overviewQueue').addEventListener('click', function (e) {
      if (!e.target.closest('button[data-goto]')) return;
      document.querySelectorAll('#consoleNav button').forEach(function (b) { b.classList.toggle('active', b.getAttribute('data-view') === 'orders'); });
      document.querySelectorAll('.console-view').forEach(function (v) { v.classList.toggle('active', v.getAttribute('data-view') === 'orders'); });
      document.getElementById('oStatus').value = 'pending';
      state.status = 'pending';
      loadBoosters().then(loadOrders);
    });

    document.getElementById('oStatus').addEventListener('change', function () { state.status = this.value; loadOrders(); });
    var timer = null;
    document.getElementById('oSearch').addEventListener('input', function () {
      var v = this.value.trim(); clearTimeout(timer);
      timer = setTimeout(function () { state.q = v; loadOrders(); }, 250);
    });
    document.getElementById('oReload').addEventListener('click', loadOrders);

    async function orderAction(btn) {
      var id = btn.getAttribute('data-id');
      try {
        var act = btn.getAttribute('data-oact');
        if (act === 'pay') await API.put('/api/admin/orders/' + encodeURIComponent(id) + '/payment', { action: 'confirm' });
        else await API.put('/api/admin/orders/' + encodeURIComponent(id), { status: act });
        UI.toast('已更新', 'ok');
        loadOrders(); loadOverview();
      } catch (err) { UI.toast(err.message, 'err'); }
    }

    async function assignOrder(sel) {
      try {
        await API.put('/api/admin/orders/' + encodeURIComponent(sel.getAttribute('data-assign')) + '/assign', { userId: sel.value });
        UI.toast(sel.value ? '已派单' : '已取消派单', 'ok');
        loadOrders();
      } catch (err) { UI.toast(err.message, 'err'); sel.value = ''; }
    }

    ['oRows', 'orderCards'].forEach(function (boxId) {
      document.getElementById(boxId).addEventListener('click', function (e) {
        var btn = e.target.closest('button[data-oact]');
        if (btn) orderAction(btn);
      });
      document.getElementById(boxId).addEventListener('change', function (e) {
        var sel = e.target.closest('select[data-assign]');
        if (sel) assignOrder(sel);
      });
    });

    document.getElementById('pRows').addEventListener('click', async function (e) {
      var btn = e.target.closest('button[data-pact]');
      if (!btn) return;
      var id = btn.getAttribute('data-id');
      var product = state.products.find(function (p) { return String(p.id) === String(id); });
      try {
        if (btn.getAttribute('data-pact') === 'price') {
          var v = prompt('新价格（元）', product.price);
          if (v === null) return;
          await API.put('/api/admin/products/' + id, { price: Number(v) });
        } else {
          await API.put('/api/admin/products/' + id, { status: btn.getAttribute('data-pact') });
        }
        UI.toast('商品已更新', 'ok');
        loadProducts();
      } catch (err) { UI.toast(err.message, 'err'); }
    });

    document.getElementById('userForm').addEventListener('submit', async function (e) {
      e.preventDefault();
      try {
        await API.post('/api/admin/users', {
          name: document.getElementById('uName').value.trim(),
          username: document.getElementById('uUser').value.trim(),
          password: document.getElementById('uPass').value,
          role: document.getElementById('uRole').value,
          share: Number(document.getElementById('uShare').value) || 0
        });
        UI.toast('账号已创建', 'ok');
        this.reset();
        document.getElementById('uShare').value = 60;
        loadUsers();
      } catch (err) { UI.toast(err.message, 'err'); }
    });
    document.getElementById('uRows').addEventListener('click', async function (e) {
      var btn = e.target.closest('button[data-uact]');
      if (!btn) return;
      var id = btn.getAttribute('data-id');
      try {
        if (btn.getAttribute('data-uact') === 'pass') {
          var pass = prompt('新密码，至少 6 位');
          if (!pass) return;
          await API.put('/api/admin/users/' + id, { password: pass });
        } else {
          await API.put('/api/admin/users/' + id, { status: btn.getAttribute('data-uact') });
        }
        UI.toast('已更新', 'ok');
        loadUsers();
      } catch (err) { UI.toast(err.message, 'err'); }
    });

    document.getElementById('settingsForm').addEventListener('submit', async function (e) {
      e.preventDefault();
      try {
        await API.put('/api/admin/settings', {
          siteName: document.getElementById('s-siteName').value.trim(),
          company: document.getElementById('s-company').value.trim(),
          notice: document.getElementById('s-notice').value.trim(),
          orderTips: document.getElementById('s-orderTips').value.trim(),
          kefu: {
            qq: document.getElementById('s-kefu-qq').value.trim(),
            wechat: document.getElementById('s-kefu-wechat').value.trim(),
            phone: (state.settings.kefu && state.settings.kefu.phone) || '',
            hours: (state.settings.kefu && state.settings.kefu.hours) || ''
          }
        });
        UI.toast('文案已保存', 'ok');
      } catch (err) { UI.toast(err.message, 'err'); }
    });
  }

  async function init() {
    bind();
    await STATION.guard(['owner', 'manager'], function (me) {
      if (me.role === 'owner') { location.href = '/admin/'; return; }
      loadBoosters().then(loadOrders);
      loadOverview();
    });
  }

  init();
})();
