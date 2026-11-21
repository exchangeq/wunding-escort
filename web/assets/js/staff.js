/* 打手端
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';
  var UI = window.UI;
  var state = { scope: 'pool', tasks: [], summary: null, current: null, image: '' };

  function money(v) { return '¥' + UI.money(v); }

  function cards() {
    var s = state.summary || {};
    var items = [
      ['可接单', s.pool || 0, '已付款、没派出去'],
      ['进行中', s.processing || 0, '手上的活'],
      ['本月完成', s.monthDone || 0, '单'],
      ['预计分成', money(s.estimate || 0), '按 ' + (s.share || 0) + '% 算']
    ];
    document.getElementById('kpis').innerHTML = items.map(function (it) {
      return '<div class="console-card"><span>' + it[0] + '</span><b>' + UI.escape(it[1]) + '</b><i>' + it[2] + '</i></div>';
    }).join('');
  }

  function paint() {
    var list = document.getElementById('taskList');
    var empty = document.getElementById('taskEmpty');
    document.getElementById('poolCount').textContent = (state.summary && state.summary.pool) || 0;
    document.getElementById('mineCount').textContent = (state.summary && state.summary.assigned) || 0;
    if (!state.tasks.length) {
      list.innerHTML = '';
      empty.style.display = 'block';
      return;
    }
    empty.style.display = 'none';
    list.innerHTML = state.tasks.map(function (o) {
      var action = state.scope === 'pool'
        ? '<button class="btn btn-primary btn-sm btn-block" data-claim="' + o.id + '">接这单</button>'
        : '<button class="btn btn-ghost btn-sm btn-block" data-open="' + o.id + '">看详情 / 报进度</button>';
      return '<article class="task-card">' +
        '<div class="task-head"><span class="mono">' + UI.escape(o.orderNo) + '</span>' + UI.status(o.status) + '</div>' +
        '<h4>' + UI.escape(o.productTitle) + '</h4>' +
        '<div class="task-meta">' +
        '<span>' + UI.escape(o.game || '') + ' · ' + UI.escape(o.server || '') + '</span>' +
        '<span>数量 ' + o.qty + '</span>' +
        '<span class="money">' + money(o.amount) + '</span>' +
        '</div>' +
        '<div class="task-sub">客户 ' + UI.escape(o.contactName) + '　' + UI.escape(o.contactPhone || '') + '</div>' +
        '<div class="task-sub">账号 ' + UI.escape(o.gameAccount || '') + '</div>' +
        (o.note ? '<div class="task-note">' + UI.escape(o.note) + '</div>' : '') +
        action +
        '</article>';
    }).join('');
  }

  async function loadSummary() {
    try {
      state.summary = await API.get('/api/staff/summary');
      cards();
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function loadTasks() {
    try {
      var data = await API.get('/api/staff/orders?scope=' + state.scope);
      state.tasks = data.items;
      paint();
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function claim(id) {
    try {
      await API.post('/api/staff/orders/' + encodeURIComponent(id) + '/claim', {});
      UI.toast('接单成功，记得先跟客户确认时间', 'ok');
      await loadSummary();
      setScope('mine');
      loadTasks();
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  function progressList(order) {
    var items = order.progress || [];
    if (!items.length) return '<div class="sub">还没有进度记录</div>';
    return '<ul class="progress-list">' + items.map(function (p) {
      return '<li><b>' + UI.date(p.at) + ' · ' + UI.escape(p.byName || '') + '</b><span>' + UI.escape(p.text) + '</span>' +
        (p.image ? '<a href="' + UI.escape(p.image) + '" target="_blank" rel="noopener">查看图片</a>' : '') + '</li>';
    }).join('') + '</ul>';
  }

  async function open(id) {
    try {
      var o = await API.get('/api/staff/orders/' + encodeURIComponent(id));
      state.current = o;
      state.image = '';
      document.getElementById('taskTitle').textContent = o.productTitle;
      document.getElementById('taskBody').innerHTML =
        '<div class="detail-grid">' +
        '<div><span>订单号</span><b class="mono">' + UI.escape(o.orderNo) + '</b></div>' +
        '<div><span>金额</span><b class="money">' + money(o.amount) + '</b></div>' +
        '<div><span>客户</span><b>' + UI.escape(o.contactName) + '　' + UI.escape(o.contactPhone) + '</b></div>' +
        '<div><span>游戏账号</span><b>' + UI.escape(o.gameAccount) + '</b></div>' +
        '<div><span>区服</span><b>' + UI.escape(o.server || '—') + '</b></div>' +
        '<div><span>状态</span><b>' + UI.statusText(o.status) + '</b></div>' +
        '</div>' +
        '<div class="detail-block"><span>客户备注</span><p>' + UI.escape(o.note || '无') + '</p></div>' +
        '<div class="detail-block"><span>进度</span>' + progressList(o) + '</div>';
      var actions = '';
      if (o.status === 'processing') actions = '<button class="btn btn-primary" id="markDone">标记完成</button>';
      if (o.status === 'pending') actions = '<button class="btn btn-primary" id="markStart">开始打</button>';
      document.getElementById('taskActions').innerHTML = actions;
      var done = document.getElementById('markDone');
      if (done) done.addEventListener('click', function () { setStatus('done'); });
      var start = document.getElementById('markStart');
      if (start) start.addEventListener('click', function () { setStatus('processing'); });
      document.getElementById('taskMask').classList.add('show');
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function setStatus(status) {
    if (!state.current) return;
    if (status === 'done' && !confirm('确认这单已经打完？确认后客户和客服都能看到完成。')) return;
    try {
      await API.post('/api/staff/orders/' + encodeURIComponent(state.current.id) + '/status', { status: status });
      UI.toast(status === 'done' ? '已标记完成' : '已开始', 'ok');
      document.getElementById('taskMask').classList.remove('show');
      loadSummary();
      loadTasks();
    } catch (err) { UI.toast(err.message, 'err'); }
  }

  async function sendProgress() {
    if (!state.current) return;
    var text = document.getElementById('progressText').value.trim();
    if (!text) return UI.toast('先写一句进度', 'err');
    var btn = document.getElementById('progressSend');
    btn.disabled = true;
    try {
      await API.post('/api/staff/orders/' + encodeURIComponent(state.current.id) + '/progress', {
        text: text, image: state.image
      });
      UI.toast('进度已提交，客户也能看到', 'ok');
      await loadSummary();
      document.getElementById('progressText').value = '';
      document.getElementById('progressFile').value = '';
      state.image = '';
      open(state.current.id);
      loadTasks();
    } catch (err) { UI.toast(err.message, 'err'); }
    finally { btn.disabled = false; }
  }

  function bind() {
    document.getElementById('tabs').addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-scope]');
      if (!btn) return;
      state.scope = btn.getAttribute('data-scope');
      document.querySelectorAll('#tabs button').forEach(function (b) { b.classList.toggle('active', b === btn); });
      loadTasks();
    });
    document.getElementById('taskList').addEventListener('click', function (e) {
      var claimBtn = e.target.closest('button[data-claim]');
      if (claimBtn) { claim(claimBtn.getAttribute('data-claim')); return; }
      var openBtn = e.target.closest('button[data-open]');
      if (openBtn) open(openBtn.getAttribute('data-open'));
    });
    document.getElementById('progressSend').addEventListener('click', sendProgress);
    document.getElementById('progressFile').addEventListener('change', function (e) {
      var file = e.target.files && e.target.files[0];
      if (!file) return;
      if (file.size > 3 * 1024 * 1024) { UI.toast('图片别超过 3MB', 'err'); return; }
      var reader = new FileReader();
      reader.onload = function () { state.image = reader.result; UI.toast('图片已选好，提交时一起发', 'ok'); };
      reader.readAsDataURL(file);
    });
    document.getElementById('taskClose').addEventListener('click', function () {
      document.getElementById('taskMask').classList.remove('show');
    });
    document.getElementById('taskMask').addEventListener('click', function (e) {
      if (e.target === this) this.classList.remove('show');
    });
  }

  function setScope(scope) {
    state.scope = scope;
    document.querySelectorAll('#tabs button').forEach(function (b) {
      b.classList.toggle('active', b.getAttribute('data-scope') === scope);
    });
  }

  async function refresh() {
    await loadSummary();
    var s = state.summary || {};
    if (!s.pool && s.assigned) setScope('mine');
    paint();
    await loadTasks();
  }

  async function init() {
    bind();
    await STATION.guard(['booster'], refresh);
  }

  init();
})();
