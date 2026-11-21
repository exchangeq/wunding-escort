/* 首页：站点信息加载、筛选与商品渲染
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var UI = window.UI;
  var state = { game: '全部', category: '全部', q: '', sort: 'default' };
  var grid = document.getElementById('productGrid');
  var gameTabs = document.getElementById('gameTabs');
  var categoryTabs = document.getElementById('categoryTabs');
  var sortSelect = document.getElementById('sortSelect');

  function renderHeroStats(stats) {
    var wrap = document.getElementById('heroStats');
    if (!wrap || !Array.isArray(stats)) return;
    wrap.innerHTML = stats
      .map(function (item) {
        return (
          '<div class="stat-card"><b>' +
          UI.escape(item.value) +
          '<span>' +
          UI.escape(item.suffix) +
          '</span></b><label>' +
          UI.escape(item.label) +
          '</label></div>'
        );
      })
      .join('');
  }

  function chip(label, count, active, type) {
    var extra = count !== undefined ? ' <small style="opacity:.65">' + count + '</small>' : '';
    return (
      '<button class="chip ' + (active ? 'active' : '') + '" data-type="' + type + '" data-value="' +
      UI.escape(label) + '">' + UI.escape(label) + extra + '</button>'
    );
  }

  function renderTabs(site) {
    gameTabs.innerHTML =
      '<span class="row-label">游戏</span>' +
      chip('全部', site.productCount, state.game === '全部', 'game') +
      site.games
        .map(function (g) { return chip(g.name, g.count, state.game === g.name, 'game'); })
        .join('');

    categoryTabs.innerHTML =
      '<span class="row-label">类型</span>' +
      chip('全部', undefined, state.category === '全部', 'category') +
      site.categories
        .map(function (c) { return chip(c, undefined, state.category === c, 'category'); })
        .join('');
  }

  function card(p) {
    var statusBadge = p.status === 'off' ? '<span class="badge badge-off">已下架</span>' : '';
    var hotBadge = (p.sales || 0) > 2000 ? '<span class="badge badge-hot">热销</span>' : '';
    var old = p.oldPrice && Number(p.oldPrice) > Number(p.price)
      ? '<span class="price-old">¥' + UI.money(p.oldPrice) + '</span>'
      : '';
    var tags = (p.tags || [])
      .slice(0, 3)
      .map(function (t) { return '<span class="tag">' + UI.escape(t) + '</span>'; })
      .join('');

    return (
      '<article class="product-card">' +
      '<div class="cover-wrap">' +
      '<img src="' + UI.escape(p.cover) + '" alt="' + UI.escape(p.title) + '" loading="lazy" />' +
      '<div class="cover-badges"><span class="badge badge-game">' + UI.escape(p.game) + '</span>' +
      '<span class="badge badge-cat">' + UI.escape(p.category) + '</span>' + hotBadge + statusBadge + '</div>' +
      '</div>' +
      '<div class="product-body">' +
      '<h3><a href="/product.html?id=' + encodeURIComponent(p.id) + '">' + UI.escape(p.title) + '</a></h3>' +
      '<p class="product-sub">' + UI.escape(p.subtitle || '') + '</p>' +
      '<div class="tag-list">' + tags + '</div>' +
      '<div class="price-row">' +
      '<div><span class="price"><span class="cur">¥</span>' + UI.money(p.price) +
      '<span class="unit">/' + UI.escape(p.unit) + '</span></span>' + old + '</div>' +
      '<span class="sales">已售 ' + Number(p.sales || 0).toLocaleString('zh-CN') + '</span>' +
      '</div>' +
      '<a class="btn btn-ghost btn-block btn-sm" href="/product.html?id=' +
      encodeURIComponent(p.id) + '">查看详情 / 下单</a>' +
      '</div></article>'
    );
  }

  async function loadProducts() {
    grid.innerHTML = '<div class="empty" style="grid-column: 1 / -1"><span class="spinner"></span> 正在加载…</div>';
    try {
      var data = await API.get(
        '/api/products' + UI.params({ game: state.game, category: state.category, q: state.q, sort: state.sort })
      );
      if (!data.items.length) {
        grid.innerHTML = '<div class="empty" style="grid-column: 1 / -1">没搜到，换个词或者直接找客服问</div>';
        return;
      }
      grid.innerHTML = data.items.map(card).join('');
    } catch (err) {
      grid.innerHTML =
        '<div class="empty" style="grid-column: 1 / -1">加载失败：' + UI.escape(err.message) + '<br /><small>请确认后端服务已启动</small></div>';
    }
  }

  function bindTabs() {
    document.addEventListener('click', function (e) {
      var btn = e.target.closest('.chip');
      if (!btn) return;
      var type = btn.getAttribute('data-type');
      var value = btn.getAttribute('data-value');
      if (type === 'game') state.game = value;
      if (type === 'category') state.category = value;
      renderTabs(window.__site || { games: [], categories: [], productCount: 0 });
      loadProducts();
    });
  }

  function bindSearch() {
    var timer = null;
    var input = document.getElementById('searchInput');
    input.addEventListener('input', function () {
      clearTimeout(timer);
      timer = setTimeout(function () {
        state.q = input.value.trim();
        loadProducts();
      }, 260);
    });
    sortSelect.addEventListener('change', function () {
      state.sort = sortSelect.value;
      loadProducts();
    });
  }

  async function init() {
    bindTabs();
    bindSearch();
    loadProducts();
    try {
      var site = await API.get('/api/site');
      window.__site = site;
      UI.hydrateSite(site.settings);
      renderHeroStats(site.settings.heroStats);
      renderTabs(site);
    } catch (err) {
      gameTabs.innerHTML = '<span class="hint">站点信息加载失败：' + UI.escape(err.message) + '</span>';
    }
  }

  init();
})();
