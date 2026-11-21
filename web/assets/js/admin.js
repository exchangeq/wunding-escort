/* 管理后台逻辑
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var UI = window.UI;
  var state = {
    view: 'overview',
    products: [],
    orders: [],
    settings: null,
    editingId: null,
    cover: ''
  };

  var VIEW_META = {
    overview: ['数据总览', '实时掌握平台经营与订单情况'],
    products: ['商品管理', '上架、编辑、下架护航服务，支持图片上传'],
    orders: ['订单管理', '接单、推进状态、处理售后与删单'],
    settings: ['站点设置', '品牌文案、客服信息、版权与首页数据'],
    security: ['账号安全', '修改后台登录密码']
  };

  /* ------------------------------ 登录态 ------------------------------ */
  function showLogin() {
    document.getElementById('loginView').style.display = 'grid';
    document.getElementById('adminView').style.display = 'none';
  }

  function showAdmin() {
    document.getElementById('loginView').style.display = 'none';
    document.getElementById('adminView').style.display = 'grid';
    loadMe();
    switchView('overview');
  }

  async function loadMe() {
    try {
      var me = await API.get('/api/admin/me');
      document.getElementById('whoName').textContent = me.displayName || me.username;
      document.querySelectorAll('[data-site="company"]').forEach(function (el) {
        el.textContent = me.company || '无定科技';
      });
    } catch (err) {
      /* ignore */
    }
  }

  async function login(e) {
    e.preventDefault();
    var btn = document.getElementById('loginBtn');
    var error = document.getElementById('loginError');
    error.textContent = '';
    btn.disabled = true;
    btn.textContent = '登录中…';
    try {
      var data = await API.post('/api/admin/login', {
        username: document.getElementById('username').value.trim(),
        password: document.getElementById('password').value
      });
      API.setToken(data.token);
      UI.toast('登录成功，欢迎回来', 'ok');
      showAdmin();
    } catch (err) {
      error.textContent = err.message;
    } finally {
      btn.disabled = false;
      btn.textContent = '登 录';
    }
  }

  function logout() {
    API.setToken('');
    showLogin();
    UI.toast('已退出登录', '');
  }

  /* ------------------------------ 视图切换 ------------------------------ */
  function switchView(view) {
    state.view = view;
    document.querySelectorAll('.side-nav button').forEach(function (btn) {
      btn.classList.toggle('active', btn.getAttribute('data-view') === view);
    });
    document.querySelectorAll('.admin-view').forEach(function (el) {
      el.classList.toggle('active', el.getAttribute('data-view') === view);
    });
    var meta = VIEW_META[view] || ['', ''];
    document.getElementById('viewTitle').textContent = meta[0];
    document.getElementById('viewSub').textContent = meta[1];

    if (view === 'overview') loadOverview();
    if (view === 'products') loadProducts();
    if (view === 'orders') loadOrders();
    if (view === 'settings') loadSettings();
  }

  /* ------------------------------ 总览 ------------------------------ */
  async function loadOverview() {
    var grid = document.getElementById('kpiGrid');
    grid.innerHTML = '<div class="empty" style="grid-column:1/-1;padding:24px"><span class="spinner"></span></div>';
    try {
      var data = await API.get('/api/admin/overview');
      var t = data.totals;
      grid.innerHTML = [
        ['订单总数', t.orders, '单'],
        ['待接单', t.pending, '单'],
        ['护航中', t.processing, '单'],
        ['已完成', t.done, '单'],
        ['今日新增', t.todayOrders, '单'],
        ['已完成收入', '¥' + UI.money(t.revenue), ''],
        ['在售商品', t.onSale + ' / ' + t.products, '个']
      ]
        .map(function (item, i) {
          return (
            '<div class="kpi"><label>' + item[0] + '</label><b style="' +
            (i === 5 ? 'color:#ffc75a' : '') + '">' + UI.escape(item[1]) +
            (item[2] ? '<small>' + item[2] + '</small>' : '') + '</b></div>'
          );
        })
        .join('');

      var max = Math.max.apply(null, data.series.map(function (s) { return s.count; }).concat([1]));
      document.getElementById('chart').innerHTML = data.series
        .map(function (s) {
          var h = Math.max(6, Math.round((s.count / max) * 120));
          return (
            '<div class="chart-col"><strong style="font-size:12px">' + s.count + '</strong>' +
            '<div class="chart-bar" style="height:' + h + 'px"></div>' +
            '<span>' + UI.escape(s.date) + '</span></div>'
          );
        })
        .join('');

      document.getElementById('recentOrders').innerHTML = data.recentOrders.length
        ? data.recentOrders
            .map(function (o) {
              return (
                '<tr><td style="color:#7fe3ff">' + UI.escape(o.orderNo) + '</td>' +
                '<td>' + UI.escape(o.productTitle) + '</td>' +
                '<td>' + UI.escape(o.contactName) + '</td>' +
                '<td>¥' + UI.money(o.amount) + '</td>' +
                '<td>' + UI.status(o.status) + '</td>' +
                '<td>' + UI.date(o.createdAt) + '</td></tr>'
              );
            })
            .join('')
        : '<tr><td colspan="6" style="text-align:center;color:#8fa1c4;padding:22px">暂无订单</td></tr>';

      document.getElementById('logLine').textContent = data.logs.length
        ? '最新：' + data.logs[0].action
        : '';
    } catch (err) {
      grid.innerHTML = '<div class="empty" style="grid-column:1/-1">' + UI.escape(err.message) + '</div>';
    }
  }

  /* ------------------------------ 商品 ------------------------------ */
  async function loadProducts() {
    var q = document.getElementById('prodSearch').value.trim();
    var game = document.getElementById('prodGame').value;
    var rows = document.getElementById('productRows');
    rows.innerHTML = '<tr><td colspan="10" style="text-align:center;padding:20px">加载中…</td></tr>';
    try {
      var data = await API.get('/api/admin/products' + UI.params({ q: q, game: game }));
      state.products = data.items;
      document.getElementById('productCount').textContent = '共 ' + data.total + ' 个';

      var gameSelect = document.getElementById('prodGame');
      var games = Array.from(new Set(data.items.map(function (p) { return p.game; })));
      var current = gameSelect.value;
      gameSelect.innerHTML =
        '<option value="全部">全部游戏</option>' +
        games.map(function (g) { return '<option value="' + UI.escape(g) + '">' + UI.escape(g) + '</option>'; }).join('');
      gameSelect.value = current && (current === '全部' || games.indexOf(current) > -1) ? current : '全部';

      rows.innerHTML = data.items.length
        ? data.items
            .map(function (p) {
              return (
                '<tr>' +
                '<td><img class="thumb" src="' + UI.escape(p.cover) + '" alt="" /></td>' +
                '<td style="min-width:220px"><b>' + UI.escape(p.title) + '</b><div class="hint">' + UI.escape(p.subtitle || '') + '</div></td>' +
                '<td>' + UI.escape(p.game) + '</td>' +
                '<td>' + UI.escape(p.category) + '</td>' +
                '<td style="color:#ffc75a;font-weight:700">¥' + UI.money(p.price) + '<small class="hint">/' + UI.escape(p.unit) + '</small></td>' +
                '<td>' + p.stock + '</td>' +
                '<td>' + Number(p.sales || 0) + '</td>' +
                '<td>' + (p.status === 'off' ? '<span class="badge badge-off">已下架</span>' : '<span class="badge badge-game">在售</span>') + '</td>' +
                '<td>' + p.sort + '</td>' +
                '<td><div class="td-actions">' +
                '<button class="btn btn-ghost btn-sm" data-act="edit" data-id="' + p.id + '">编辑</button>' +
                '<button class="btn btn-ghost btn-sm" data-act="toggle" data-id="' + p.id + '">' + (p.status === 'off' ? '上架' : '下架') + '</button>' +
                '<button class="btn btn-sm btn-danger" data-act="del" data-id="' + p.id + '">删除</button>' +
                '</div></td>' +
                '</tr>'
              );
            })
            .join('')
        : '<tr><td colspan="10" style="text-align:center;color:#8fa1c4;padding:26px">还没有商品，点击右上角「新增商品」开始上架</td></tr>';
    } catch (err) {
      rows.innerHTML = '<tr><td colspan="10" style="text-align:center;color:#ff6b81;padding:22px">' + UI.escape(err.message) + '</td></tr>';
    }
  }

  function setCover(url) {
    state.cover = url || '';
    document.getElementById('p-cover').value = state.cover;
    var preview = document.getElementById('coverPreview');
    preview.innerHTML = state.cover
      ? '<img src="' + UI.escape(state.cover) + '" alt="封面预览" />'
      : '<span>暂无封面</span>';
  }

  function openEditor(product) {
    state.editingId = product ? product.id : null;
    document.getElementById('productModalTitle').textContent = product ? '编辑商品 · ' + product.title : '新增商品';
    document.getElementById('productError').textContent = '';
    document.getElementById('productForm').reset();

    var p = product || {
      title: '', subtitle: '', game: '', category: '上分护航', unit: '局',
      price: '', oldPrice: '', stock: 10, sales: 0, sort: 0, status: 'on',
      tags: [], includes: [], desc: '', cover: ''
    };

    document.getElementById('p-title').value = p.title || '';
    document.getElementById('p-subtitle').value = p.subtitle || '';
    document.getElementById('p-game').value = p.game || '';
    document.getElementById('p-category').value = p.category || '上分护航';
    document.getElementById('p-unit').value = p.unit || '局';
    document.getElementById('p-price').value = p.price === undefined || p.price === null ? '' : p.price;
    document.getElementById('p-oldPrice').value = p.oldPrice || '';
    document.getElementById('p-stock').value = p.stock === undefined ? 10 : p.stock;
    document.getElementById('p-sales').value = p.sales || 0;
    document.getElementById('p-sort').value = p.sort || 0;
    document.getElementById('p-status').value = p.status === 'off' ? 'off' : 'on';
    document.getElementById('p-tags').value = (p.tags || []).join(',');
    document.getElementById('p-includes').value = (p.includes || []).join('\n');
    document.getElementById('p-desc').value = p.desc || '';
    setCover(p.cover || '');

    document.getElementById('productMask').classList.add('show');
  }

  function closeEditor() {
    document.getElementById('productMask').classList.remove('show');
    state.editingId = null;
  }

  async function saveProduct(e) {
    e.preventDefault();
    var error = document.getElementById('productError');
    var btn = document.getElementById('productSave');
    error.textContent = '';

    var payload = {
      title: document.getElementById('p-title').value.trim(),
      subtitle: document.getElementById('p-subtitle').value.trim(),
      game: document.getElementById('p-game').value.trim(),
      category: document.getElementById('p-category').value.trim(),
      unit: document.getElementById('p-unit').value.trim() || '局',
      price: Number(document.getElementById('p-price').value || 0),
      oldPrice: Number(document.getElementById('p-oldPrice').value || 0),
      stock: Number(document.getElementById('p-stock').value || 0),
      sales: Number(document.getElementById('p-sales').value || 0),
      sort: Number(document.getElementById('p-sort').value || 0),
      status: document.getElementById('p-status').value,
      tags: document.getElementById('p-tags').value.split(/[,，]/).map(function (s) { return s.trim(); }).filter(Boolean),
      includes: document.getElementById('p-includes').value.split('\n').map(function (s) { return s.trim(); }).filter(Boolean),
      desc: document.getElementById('p-desc').value.trim(),
      cover: document.getElementById('p-cover').value.trim() || state.cover
    };

    if (!payload.title) return (error.textContent = '请填写商品标题');
    if (!payload.game) return (error.textContent = '请填写所属游戏');
    if (!payload.category) return (error.textContent = '请填写服务类型');
    if (payload.price <= 0) return (error.textContent = '价格必须大于 0');

    btn.disabled = true;
    btn.textContent = '保存中…';
    try {
      if (state.editingId) {
        await API.put('/api/admin/products/' + state.editingId, payload);
      } else {
        await API.post('/api/admin/products', payload);
      }
      UI.toast('商品已保存', 'ok');
      closeEditor();
      loadProducts();
    } catch (err) {
      error.textContent = err.message;
    } finally {
      btn.disabled = false;
      btn.textContent = '保存商品';
    }
  }

  async function handleProductAction(act, id) {
    var product = state.products.find(function (p) { return Number(p.id) === Number(id); });
    if (act === 'edit') return openEditor(product);
    if (!product) return;
    if (act === 'toggle') {
      try {
        await API.put('/api/admin/products/' + id, Object.assign({}, product, {
          status: product.status === 'off' ? 'on' : 'off'
        }));
        UI.toast(product.status === 'off' ? '已上架' : '已下架', 'ok');
        loadProducts();
      } catch (err) {
        UI.toast(err.message, 'err');
      }
    }
    if (act === 'del') {
      if (!confirm('确定删除商品「' + product.title + '」？此操作不可恢复。')) return;
      try {
        await API.del('/api/admin/products/' + id);
        UI.toast('商品已删除', 'ok');
        loadProducts();
      } catch (err) {
        UI.toast(err.message, 'err');
      }
    }
  }

  async function uploadCover(file) {
    if (!file) return;
    if (file.size > 5 * 1024 * 1024) return UI.toast('图片不能超过 5MB', 'err');
    var reader = new FileReader();
    reader.onload = async function () {
      try {
        var data = await API.post('/api/admin/upload', { data: reader.result, name: file.name });
        setCover(data.url);
        UI.toast('封面已上传', 'ok');
      } catch (err) {
        UI.toast(err.message, 'err');
      }
    };
    reader.readAsDataURL(file);
  }

  /* ------------------------------ 订单 ------------------------------ */
  async function loadOrders() {
    var rows = document.getElementById('orderRows');
    rows.innerHTML = '<tr><td colspan="10" style="text-align:center;padding:20px">加载中…</td></tr>';
    try {
      var data = await API.get('/api/admin/orders' + UI.params({
        status: document.getElementById('orderStatus').value,
        q: document.getElementById('orderSearch').value.trim()
      }));
      state.orders = data.items;
      document.getElementById('orderCount').textContent = '共 ' + data.total + ' 单';
      rows.innerHTML = data.items.length
        ? data.items
            .map(function (o) {
              var actions = '';
              if (o.status === 'pending')
                actions += '<button class="btn btn-ghost btn-sm" data-oact="processing" data-id="' + o.id + '">接单</button>';
              if (o.status === 'processing' || o.status === 'pending')
                actions += '<button class="btn btn-ghost btn-sm" data-oact="done" data-id="' + o.id + '">完成</button>';
              if (o.status !== 'done' && o.status !== 'cancelled')
                actions += '<button class="btn btn-ghost btn-sm" data-oact="cancelled" data-id="' + o.id + '">取消</button>';
              actions += '<button class="btn btn-sm btn-danger" data-oact="del" data-id="' + o.id + '">删除</button>';
              return (
                '<tr>' +
                '<td style="color:#7fe3ff;white-space:nowrap">' + UI.escape(o.orderNo) + '</td>' +
                '<td style="min-width:180px">' + UI.escape(o.productTitle) + '</td>' +
                '<td>' + o.qty + '</td>' +
                '<td style="color:#ffc75a">¥' + UI.money(o.amount) + '</td>' +
                '<td>' + UI.escape(o.contactName) + '<div class="hint">' + UI.escape(o.contactPhone) + '</div></td>' +
                '<td>' + UI.escape(o.gameAccount) + '<div class="hint">' + UI.escape(o.server || '') + '</div></td>' +
                '<td style="max-width:180px;color:#8fa1c4;font-size:12px">' + UI.escape(o.note || '—') + '</td>' +
                '<td>' + UI.status(o.status) + '</td>' +
                '<td style="white-space:nowrap">' + UI.date(o.createdAt) + '</td>' +
                '<td><div class="td-actions">' + actions + '</div></td>' +
                '</tr>'
              );
            })
            .join('')
        : '<tr><td colspan="10" style="text-align:center;color:#8fa1c4;padding:26px">暂无订单</td></tr>';
    } catch (err) {
      rows.innerHTML = '<tr><td colspan="10" style="text-align:center;color:#ff6b81;padding:22px">' + UI.escape(err.message) + '</td></tr>';
    }
  }

  async function handleOrderAction(act, id) {
    try {
      if (act === 'del') {
        if (!confirm('确定删除该订单记录？')) return;
        await API.del('/api/admin/orders/' + encodeURIComponent(id));
        UI.toast('订单已删除', 'ok');
      } else {
        await API.put('/api/admin/orders/' + encodeURIComponent(id), { status: act });
        UI.toast('订单状态已更新为「' + UI.statusText(act) + '」', 'ok');
      }
      loadOrders();
    } catch (err) {
      UI.toast(err.message, 'err');
    }
  }

  /* ------------------------------ 设置 ------------------------------ */
  function heroFields(stats) {
    var wrap = document.getElementById('heroStatFields');
    var data = (stats && stats.length ? stats : [0, 1, 2, 3].map(function () {
      return { label: '', value: '', suffix: '' };
    })).slice(0, 4);
    while (data.length < 4) data.push({ label: '', value: '', suffix: '' });
    wrap.innerHTML = data
      .map(function (item, i) {
        return (
          '<div class="admin-panel" style="padding:12px;margin:0">' +
          '<div class="hint" style="margin-bottom:8px">数据卡片 ' + (i + 1) + '</div>' +
          '<div class="form-grid-3">' +
          '<input id="hero-label-' + i + '" type="text" placeholder="标签，如 认证护航师" value="' + UI.escape(item.label || '') + '" />' +
          '<input id="hero-value-' + i + '" type="text" placeholder="数值" value="' + UI.escape(item.value || '') + '" />' +
          '<input id="hero-suffix-' + i + '" type="text" placeholder="后缀，如 +" value="' + UI.escape(item.suffix || '') + '" />' +
          '</div></div>'
        );
      })
      .join('');
  }

  async function loadSettings() {
    try {
      var s = await API.get('/api/admin/settings');
      state.settings = s;
      document.getElementById('s-siteName').value = s.siteName || '';
      document.getElementById('s-company').value = s.company || '';
      document.getElementById('s-slogan').value = s.slogan || '';
      document.getElementById('s-subSlogan').value = s.subSlogan || '';
      document.getElementById('s-notice').value = s.notice || '';
      document.getElementById('s-copyright').value = s.copyright || '';
      document.getElementById('s-icp').value = s.icp || '';
      document.getElementById('s-orderTips').value = s.orderTips || '';
      document.getElementById('s-kefu-qq').value = (s.kefu && s.kefu.qq) || '';
      document.getElementById('s-kefu-wechat').value = (s.kefu && s.kefu.wechat) || '';
      document.getElementById('s-kefu-phone').value = (s.kefu && s.kefu.phone) || '';
      document.getElementById('s-kefu-hours').value = (s.kefu && s.kefu.hours) || '';
      heroFields(s.heroStats);
    } catch (err) {
      UI.toast(err.message, 'err');
    }
  }

  async function saveSettings(e) {
    e.preventDefault();
    var btn = document.getElementById('saveSettingsBtn');
    var stats = [0, 1, 2, 3].map(function (i) {
      return {
        label: document.getElementById('hero-label-' + i).value.trim(),
        value: document.getElementById('hero-value-' + i).value.trim(),
        suffix: document.getElementById('hero-suffix-' + i).value.trim()
      };
    }).filter(function (item) { return item.label || item.value; });

    var payload = {
      siteName: document.getElementById('s-siteName').value.trim(),
      company: document.getElementById('s-company').value.trim(),
      slogan: document.getElementById('s-slogan').value.trim(),
      subSlogan: document.getElementById('s-subSlogan').value.trim(),
      notice: document.getElementById('s-notice').value.trim(),
      copyright: document.getElementById('s-copyright').value.trim(),
      icp: document.getElementById('s-icp').value.trim(),
      orderTips: document.getElementById('s-orderTips').value.trim(),
      heroStats: stats,
      kefu: {
        qq: document.getElementById('s-kefu-qq').value.trim(),
        wechat: document.getElementById('s-kefu-wechat').value.trim(),
        phone: document.getElementById('s-kefu-phone').value.trim(),
        hours: document.getElementById('s-kefu-hours').value.trim()
      }
    };

    btn.disabled = true;
    btn.textContent = '保存中…';
    try {
      await API.put('/api/admin/settings', payload);
      UI.toast('站点设置已保存，前台已同步生效', 'ok');
      loadMe();
    } catch (err) {
      UI.toast(err.message, 'err');
    } finally {
      btn.disabled = false;
      btn.textContent = '保存设置';
    }
  }

  async function changePassword(e) {
    e.preventDefault();
    var oldPassword = document.getElementById('oldPassword').value;
    var newPassword = document.getElementById('newPassword').value;
    var confirmPassword = document.getElementById('newPassword2').value;
    if (newPassword !== confirmPassword) return UI.toast('两次输入的新密码不一致', 'err');
    if (newPassword.length < 6) return UI.toast('新密码至少 6 位', 'err');
    try {
      await API.post('/api/admin/password', { oldPassword: oldPassword, newPassword: newPassword });
      UI.toast('密码已更新，请重新登录', 'ok');
      document.getElementById('passwordForm').reset();
      setTimeout(logout, 1200);
    } catch (err) {
      UI.toast(err.message, 'err');
    }
  }

  /* ------------------------------ 事件绑定 ------------------------------ */
  function bind() {
    document.getElementById('loginForm').addEventListener('submit', login);
    document.getElementById('logoutBtn').addEventListener('click', logout);
    document.getElementById('refreshBtn').addEventListener('click', function () {
      switchView(state.view);
      UI.toast('数据已刷新', 'ok');
    });
    document.addEventListener('auth:expired', function () {
      UI.toast('登录已过期，请重新登录', 'err');
      showLogin();
    });

    document.getElementById('sideNav').addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-view]');
      if (btn) switchView(btn.getAttribute('data-view'));
    });

    // 商品
    document.getElementById('newProductBtn').addEventListener('click', function () { openEditor(null); });
    document.getElementById('productCancel').addEventListener('click', closeEditor);
    document.getElementById('productMask').addEventListener('click', function (e) {
      if (e.target === this) closeEditor();
    });
    document.getElementById('productForm').addEventListener('submit', saveProduct);
    document.getElementById('p-coverFile').addEventListener('change', function (e) {
      uploadCover(e.target.files && e.target.files[0]);
    });
    document.getElementById('productRows').addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-act]');
      if (btn) handleProductAction(btn.getAttribute('data-act'), btn.getAttribute('data-id'));
    });
    var prodTimer = null;
    document.getElementById('prodSearch').addEventListener('input', function () {
      clearTimeout(prodTimer);
      prodTimer = setTimeout(loadProducts, 260);
    });
    document.getElementById('prodGame').addEventListener('change', loadProducts);

    // 订单
    document.getElementById('orderStatus').addEventListener('change', loadOrders);
    document.getElementById('orderReload').addEventListener('click', loadOrders);
    var orderTimer = null;
    document.getElementById('orderSearch').addEventListener('input', function () {
      clearTimeout(orderTimer);
      orderTimer = setTimeout(loadOrders, 260);
    });
    document.getElementById('orderRows').addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-oact]');
      if (btn) handleOrderAction(btn.getAttribute('data-oact'), btn.getAttribute('data-id'));
    });

    // 设置
    document.getElementById('settingsForm').addEventListener('submit', saveSettings);
    document.getElementById('reloadSettingsBtn').addEventListener('click', loadSettings);
    document.getElementById('passwordForm').addEventListener('submit', changePassword);
  }

  function init() {
    bind();
    if (API.token) showAdmin();
    else showLogin();
  }

  init();
})();
