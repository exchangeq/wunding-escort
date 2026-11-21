/* 员工端通用：登录、角色守卫、导航
   © 2026 无定科技 版权所有 */
(function () {
  'use strict';

  var ROLE_HOME = { owner: '/admin/', manager: '/manager/', support: '/support/', booster: '/staff/' };
  var ROLE_NAME = { owner: '董事长', manager: '管事', support: '客服', booster: '打手' };

  function $(id) { return document.getElementById(id); }

  function showLogin(message) {
    var login = $('loginView');
    var app = $('appView');
    if (login) login.style.display = 'grid';
    if (app) app.style.display = 'none';
    var err = $('loginError');
    if (err && message) err.textContent = message;
  }

  function showApp() {
    var login = $('loginView');
    var app = $('appView');
    if (login) login.style.display = 'none';
    if (app) app.style.display = 'block';
  }

  async function guard(roles, onReady) {
    if (!API.token) {
      showLogin('');
      return false;
    }
    try {
      var me = await API.get('/api/auth/me');
      if (roles && roles.indexOf(me.role) === -1) {
        location.href = ROLE_HOME[me.role] || '/';
        return false;
      }
      showApp();
      paintUser(me);
      if (onReady) onReady(me);
      return true;
    } catch (err) {
      API.setToken('');
      showLogin('登录状态失效，重新登录一下');
      return false;
    }
  }

  function paintUser(user) {
    document.querySelectorAll('[data-user-name]').forEach(function (el) {
      el.textContent = user.name || user.username;
    });
    document.querySelectorAll('[data-user-role]').forEach(function (el) {
      el.textContent = ROLE_NAME[user.role] || user.role;
    });
  }

  async function login(event) {
    event.preventDefault();
    var err = $('loginError');
    err.textContent = '';
    var btn = $('loginBtn');
    btn.disabled = true;
    btn.textContent = '登录中…';
    try {
      var data = await API.post('/api/auth/login', {
        username: $('loginUser').value.trim(),
        password: $('loginPass').value
      });
      API.setToken(data.token);
      localStorage.setItem('wd_role', data.user.role);
      location.reload();
    } catch (e) {
      err.textContent = e.message;
    } finally {
      btn.disabled = false;
      btn.textContent = '登 录';
    }
  }

  function bindLogin() {
    var form = $('loginForm');
    if (form) form.addEventListener('submit', login);
  }

  function logout() {
    API.setToken('');
    location.reload();
  }

  function bindShell() {
    var nav = $('consoleNav');
    if (nav) {
      nav.addEventListener('click', function (e) {
        var btn = e.target.closest('button[data-view]');
        if (!btn) return;
        var view = btn.getAttribute('data-view');
        nav.querySelectorAll('button').forEach(function (b) { b.classList.toggle('active', b === btn); });
        document.querySelectorAll('.console-view').forEach(function (el) {
          el.classList.toggle('active', el.getAttribute('data-view') === view);
        });
        document.dispatchEvent(new CustomEvent('console:view', { detail: view }));
      });
    }
    document.querySelectorAll('[data-logout]').forEach(function (el) {
      el.addEventListener('click', logout);
    });
  }

  bindLogin();
  bindShell();

  window.STATION = {
    guard: guard,
    showLogin: showLogin,
    paintUser: paintUser,
    roleName: function (role) { return ROLE_NAME[role] || role; },
    roleHome: function (role) { return ROLE_HOME[role] || '/'; }
  };
})();
