/* 付款面板：收款码 / 在线支付 / 凭证上传
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var UI = window.UI;

  function payText(status) {
    return { unpaid: '待付款', review: '待确认', paid: '已到账', refunded: '已退款' }[status] || '待付款';
  }

  function qrSection(manual, orderNo) {
    var tabs = [];
    if (manual.wechatQr) tabs.push({ key: 'wechat', label: '微信', src: manual.wechatQr });
    if (manual.alipayQr) tabs.push({ key: 'alipay', label: '支付宝', src: manual.alipayQr });
    if (!tabs.length) {
      return '<div class="pay-note">还没放收款码，加客服微信/QQ 直接转，付款后把截图发给值班。</div>';
    }
    var buttons = tabs
      .map(function (t, i) {
        return '<button type="button" class="qr-tab' + (i === 0 ? ' active' : '') + '" data-qr="' + t.key + '">' + t.label + '</button>';
      })
      .join('');
    return (
      '<div class="qr-tabs">' + buttons + '</div>' +
      '<div class="qr-panel">' +
      tabs
        .map(function (t, i) {
          return '<img class="qr-img' + (i === 0 ? ' show' : '') + '" data-qr-img="' + t.key + '" src="' + UI.escape(t.src) + '" alt="' + t.label + '收款码" />';
        })
        .join('') +
      '<div class="qr-side">' +
      '<div class="pay-amount">¥<b>' + UI.money(0) + '</b></div>' +
      '<p>付款备注：<b class="pay-no">' + UI.escape(orderNo) + '</b></p>' +
      '<button type="button" class="btn btn-ghost btn-sm" data-copy="' + UI.escape(orderNo) + '">复制订单号</button>' +
      '</div></div>'
    );
  }

  function render(el, order, payment) {
    var status = order.payStatus || 'unpaid';
    payment = payment || {};
    var manual = payment.manual || { enabled: true };
    var epay = payment.epay || { enabled: false };
    var html = '';

    html += '<div class="pay-head"><span class="pay-state pay-' + status + '">' + payText(status) + '</span>' +
      '<span class="pay-total">应付 <b>¥' + UI.money(order.amount) + '</b></span></div>';

    if (status === 'paid') {
      html += '<div class="pay-note ok">已经确认到账' + (order.payAt ? '（' + UI.date(order.payAt) + '）' : '') +
        '，接下来按排单顺序安排护航师。</div>';
    } else if (status === 'review') {
      html += '<div class="pay-note">凭证已收到，值班核对后就开始排单。一般十几分钟，急单直接找客服。</div>';
      if (order.payProof) {
        html += '<a class="pay-proof" href="' + UI.escape(order.payProof) + '" target="_blank" rel="noopener">查看已上传的付款截图</a>';
      }
    } else if (status === 'refunded') {
      html += '<div class="pay-note">这单已退款，钱会按原路退回，到账时间看支付渠道。</div>';
    } else {
      if (manual.enabled) {
        html += qrSection(manual, order.orderNo);
        if (manual.note) html += '<div class="pay-note">' + UI.escape(manual.note) + '</div>';
        html +=
          '<div class="pay-actions">' +
          '<label class="btn btn-ghost btn-sm file-btn">上传付款截图' +
          '<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" data-proof hidden /></label>' +
          '<span class="hint" data-proof-tip>付完传一下截图，客服核对快一些</span>' +
          '</div>';
      }
      if (epay.enabled) {
        html += '<button type="button" class="btn btn-primary btn-block" data-epay>' + UI.escape(epay.name || '在线支付') + '</button>';
      }
      if (!manual.enabled && !epay.enabled) {
        html += '<div class="pay-note">线上收款暂时没开，联系客服拿收款方式。</div>';
      }
    }

    el.innerHTML = '<div class="pay-box">' + html + '</div>';
    var amountEl = el.querySelector('.pay-amount b');
    if (amountEl) amountEl.textContent = UI.money(order.amount);
    bind(el, order, payment);
  }

  function copy(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () { UI.toast('订单号已复制', 'ok'); }, fallback);
    } else {
      fallback();
    }
    function fallback() {
      var input = document.createElement('textarea');
      input.value = text;
      input.style.position = 'fixed';
      input.style.opacity = '0';
      document.body.appendChild(input);
      input.select();
      try { document.execCommand('copy'); UI.toast('订单号已复制', 'ok'); } catch (e) { UI.toast('复制失败，手动记一下 ' + text, 'err'); }
      input.remove();
    }
  }

  function bind(el, order, payment) {
    el.querySelectorAll('.qr-tab').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var key = btn.getAttribute('data-qr');
        el.querySelectorAll('.qr-tab').forEach(function (b) { b.classList.toggle('active', b === btn); });
        el.querySelectorAll('.qr-img').forEach(function (img) {
          img.classList.toggle('show', img.getAttribute('data-qr-img') === key);
        });
      });
    });

    el.querySelectorAll('[data-copy]').forEach(function (btn) {
      btn.addEventListener('click', function () { copy(btn.getAttribute('data-copy')); });
    });

    var file = el.querySelector('[data-proof]');
    if (file) {
      file.addEventListener('change', function () {
        var f = file.files && file.files[0];
        if (!f) return;
        if (f.size > 3 * 1024 * 1024) {
          UI.toast('图片别超过 3MB，压一下再传', 'err');
          return;
        }
        var reader = new FileReader();
        reader.onload = async function () {
          var tip = el.querySelector('[data-proof-tip]');
          if (tip) tip.textContent = '上传中…';
          try {
            await API.post('/api/orders/' + encodeURIComponent(order.orderNo) + '/proof', { data: reader.result });
            UI.toast('截图已上传，等客服核对', 'ok');
            order.payStatus = 'review';
            render(el, order, payment);
          } catch (err) {
            if (tip) tip.textContent = err.message;
            UI.toast(err.message, 'err');
          }
        };
        reader.readAsDataURL(f);
      });
    }

    var epayBtn = el.querySelector('[data-epay]');
    if (epayBtn) {
      epayBtn.addEventListener('click', async function () {
        epayBtn.disabled = true;
        try {
          var data = await API.post('/api/orders/' + encodeURIComponent(order.orderNo) + '/pay', {});
          if (data.payUrl) location.href = data.payUrl;
        } catch (err) {
          epayBtn.disabled = false;
          UI.toast(err.message, 'err');
        }
      });
    }
  }

  window.PAYUI = { render: render };
})();
