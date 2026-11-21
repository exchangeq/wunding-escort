/* 无定电竞护航系统 · 公共脚本
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var TOKEN_KEY = 'wd_admin_token';

  var API = {
    base: window.API_BASE || '',
    token: localStorage.getItem(TOKEN_KEY) || '',

    setToken: function (token) {
      API.token = token || '';
      if (token) localStorage.setItem(TOKEN_KEY, token);
      else localStorage.removeItem(TOKEN_KEY);
    },

    request: async function (path, options) {
      options = options || {};
      var headers = { 'Content-Type': 'application/json' };
      if (API.token) headers.Authorization = 'Bearer ' + API.token;

      var res;
      try {
        res = await fetch(API.base + path, {
          method: options.method || 'GET',
          headers: headers,
          body: options.body ? JSON.stringify(options.body) : undefined,
          cache: 'no-store'
        });
      } catch (err) {
        throw new Error('网络连接失败，请检查服务是否已启动');
      }

      var data = null;
      try {
        data = await res.json();
      } catch (err) {
        data = null;
      }

      if (!res.ok || !data || data.ok === false) {
        var message = (data && data.error && data.error.message) || '请求失败（' + res.status + '）';
        var error = new Error(message);
        error.status = res.status;
        error.code = data && data.error ? data.error.code : 'ERROR';
        if (res.status === 401 && !path.includes('/login')) {
          API.setToken('');
          document.dispatchEvent(new CustomEvent('auth:expired'));
        }
        throw error;
      }
      return data.data;
    },

    get: function (path) {
      return API.request(path);
    },
    post: function (path, body) {
      return API.request(path, { method: 'POST', body: body || {} });
    },
    put: function (path, body) {
      return API.request(path, { method: 'PUT', body: body || {} });
    },
    del: function (path) {
      return API.request(path, { method: 'DELETE' });
    }
  };

  var STATUS = {
    pending: { text: '待接单', cls: 'status-pending' },
    processing: { text: '护航中', cls: 'status-processing' },
    done: { text: '已完成', cls: 'status-done' },
    cancelled: { text: '已取消', cls: 'status-cancelled' }
  };

  var UI = {
    escape: function (value) {
      return String(value === undefined || value === null ? '' : value)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
    },

    money: function (value) {
      var n = Number(value) || 0;
      return (Math.round(n * 100) / 100).toFixed(n % 1 === 0 ? 0 : 2);
    },

    status: function (key) {
      var s = STATUS[key] || { text: key, cls: 'status-pending' };
      return '<span class="status-pill ' + s.cls + '">' + s.text + '</span>';
    },

    statusText: function (key) {
      return (STATUS[key] || { text: key }).text;
    },

    date: function (value) {
      if (!value) return '--';
      var d = new Date(value);
      if (isNaN(d.getTime())) return String(value);
      var p = function (n) { return String(n).padStart(2, '0'); };
      return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
    },

    toast: function (message, type) {
      var wrap = document.querySelector('.toast-wrap');
      if (!wrap) {
        wrap = document.createElement('div');
        wrap.className = 'toast-wrap';
        document.body.appendChild(wrap);
      }
      var el = document.createElement('div');
      el.className = 'toast ' + (type || '');
      el.textContent = message;
      wrap.appendChild(el);
      setTimeout(function () {
        el.style.opacity = '0';
        el.style.transform = 'translateY(8px)';
        el.style.transition = '0.25s ease';
        setTimeout(function () { el.remove(); }, 260);
      }, 2600);
    },

    qs: function (name) {
      return new URLSearchParams(location.search).get(name);
    },

    params: function (obj) {
      var out = [];
      Object.keys(obj || {}).forEach(function (key) {
        var value = obj[key];
        if (value === undefined || value === null || value === '') return;
        out.push(encodeURIComponent(key) + '=' + encodeURIComponent(value));
      });
      return out.length ? '?' + out.join('&') : '';
    }
  };

  /** 把站点设置填充到页面里带 data-site 属性的节点 */
  UI.hydrateSite = function (settings) {
    if (!settings) return;
    var map = {
      siteName: settings.siteName,
      company: settings.company,
      slogan: settings.slogan,
      subSlogan: settings.subSlogan,
      copyright: settings.copyright,
      icp: settings.icp,
      notice: settings.notice
    };
    Object.keys(map).forEach(function (key) {
      if (map[key] === undefined) return;
      document.querySelectorAll('[data-site="' + key + '"]').forEach(function (el) {
        el.textContent = map[key];
      });
    });
    document.querySelectorAll('[data-site="title"]').forEach(function (el) {
      document.title = settings.siteName + ' · ' + el.getAttribute('data-page');
    });
    var kefu = settings.kefu || {};
    document.querySelectorAll('[data-kefu]').forEach(function (el) {
      var key = el.getAttribute('data-kefu');
      if (kefu[key]) el.textContent = kefu[key];
    });
  };

  window.API = API;
  window.UI = UI;
})();
