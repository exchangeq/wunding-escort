/* 商品详情与下单
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var UI = window.UI;
  var product = null;
  var qtyInput = document.getElementById('qty');
  var formError = document.getElementById('formError');

  function render(p) {
    document.title = p.title + ' · ' + (document.querySelector('[data-site="siteName"]')?.textContent || '无定电竞护航');
    document.getElementById('crumbTitle').textContent = p.title;
    document.getElementById('title').textContent = p.title;
    document.getElementById('subtitle').textContent = p.subtitle || '';
    document.getElementById('cover').src = p.cover || '/assets/img/cover-default.svg';
    document.getElementById('cover').alt = p.title;
    document.getElementById('price').textContent = UI.money(p.price);
    document.getElementById('unit').textContent = '/' + p.unit;
    document.getElementById('oldPrice').textContent =
      p.oldPrice && Number(p.oldPrice) > Number(p.price) ? '原价 ¥' + UI.money(p.oldPrice) : '';
    document.getElementById('sales').textContent = Number(p.sales || 0).toLocaleString('zh-CN');
    document.getElementById('stock').textContent = p.stock === null || p.stock === undefined ? '充足' : p.stock;
    document.getElementById('desc').textContent = p.desc || '暂无详细说明，可联系客服咨询。';

    document.getElementById('badges').innerHTML =
      '<span class="badge badge-game">' + UI.escape(p.game) + '</span>' +
      '<span class="badge badge-cat">' + UI.escape(p.category) + '</span>' +
      (p.tags || []).map(function (t) { return '<span class="tag">' + UI.escape(t) + '</span>'; }).join('');

    document.getElementById('includes').innerHTML = (p.includes && p.includes.length
      ? p.includes
      : ['专业护航师一对一对接', '订单全程进度可查', '异常情况售后处理']
    )
      .map(function (item) { return '<li>' + UI.escape(item) + '</li>'; })
      .join('');

    qtyInput.max = p.stock && p.stock > 0 ? Math.min(p.stock, 50) : 50;
    updateTotal();
  }

  function updateTotal() {
    if (!product) return;
    var qty = Math.max(1, Math.min(Number(qtyInput.value) || 1, 50));
    qtyInput.value = qty;
    document.getElementById('total').textContent = UI.money(product.price * qty);
  }

  function bindQty() {
    document.querySelectorAll('.qty-control button').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var step = Number(btn.getAttribute('data-step'));
        qtyInput.value = Math.max(1, Math.min(Number(qtyInput.value) + step, 50));
        updateTotal();
      });
    });
    qtyInput.addEventListener('input', updateTotal);
  }

  async function submit(e) {
    e.preventDefault();
    formError.textContent = '';
    var btn = document.getElementById('submitBtn');
    var payload = {
      productId: product.id,
      qty: Number(qtyInput.value) || 1,
      contactName: document.getElementById('contactName').value.trim(),
      contactPhone: document.getElementById('contactPhone').value.trim(),
      gameAccount: document.getElementById('gameAccount').value.trim(),
      server: document.getElementById('server').value.trim(),
      note: document.getElementById('note').value.trim()
    };

    if (!payload.contactName) return (formError.textContent = '请填写您的称呼');
    if (!payload.contactPhone || payload.contactPhone.length < 5)
      return (formError.textContent = '请填写有效的手机号 / QQ / 微信');
    if (!payload.gameAccount) return (formError.textContent = '请填写游戏账号或角色 ID');

    btn.disabled = true;
    btn.textContent = '提交中…';
    try {
      var data = await API.post('/api/orders', payload);
      document.getElementById('okNo').textContent = data.orderNo;
      document.getElementById('okProduct').textContent = data.productTitle;
      document.getElementById('okAmount').textContent = '¥' + UI.money(data.amount);
      document.getElementById('okStatus').textContent = UI.statusText(data.status);
      document.getElementById('okTips').textContent = data.tips || '';
      document.getElementById('okQq').textContent = (data.kefu && data.kefu.qq) || '--';
      document.getElementById('okWechat').textContent = (data.kefu && data.kefu.wechat) || '--';
      document.getElementById('okTrack').href = '/track.html?no=' + encodeURIComponent(data.orderNo);
      if (window.PAYUI) PAYUI.render(document.getElementById('payBox'), data, data.payment);
      document.getElementById('successMask').classList.add('show');
      UI.toast('下单成功，订单号 ' + data.orderNo, 'ok');
    } catch (err) {
      formError.textContent = err.message;
      UI.toast(err.message, 'err');
    } finally {
      btn.disabled = false;
      btn.textContent = '提交订单';
    }
  }

  async function init() {
    bindQty();
    document.getElementById('orderForm').addEventListener('submit', submit);
    document.getElementById('successMask').addEventListener('click', function (e) {
      if (e.target === this) this.classList.remove('show');
    });

    var id = UI.qs('id');
    if (!id) {
      document.getElementById('title').textContent = '缺少商品 ID';
      return;
    }
    try {
      product = await API.get('/api/products/' + encodeURIComponent(id));
      render(product);
    } catch (err) {
      document.getElementById('title').textContent = '服务不存在或已下架';
      document.getElementById('desc').textContent = err.message;
    }
    try {
      var site = await API.get('/api/site');
      UI.hydrateSite(site.settings);
      if (site.settings && site.settings.kefu) {
        document.querySelectorAll('[data-kefu]').forEach(function (el) {
          var key = el.getAttribute('data-kefu');
          if (site.settings.kefu[key]) el.textContent = site.settings.kefu[key];
        });
      }
    } catch (err) {
      /* 忽略：不影响下单 */
    }
  }

  init();
})();
