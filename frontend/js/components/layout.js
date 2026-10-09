/* ============================================================
   layout.js —— 应用外壳
   侧边导航 + 顶部栏 + 内容区路由 + 全局提示
   ============================================================ */
(function () {
  'use strict';
  var computed = Vue.computed;

  var NAV = [
    { key: 'dashboard',  name: '工作台',        icon: 'ri-dashboard-3-line', title: '工作台总览',       crumb: '首页 / 工作台' },
    { key: 'archive',    name: '病案归档',      icon: 'ri-archive-line', title: '病案归档管理',     crumb: '首页 / 病案归档' },
    { key: 'catalog',    name: '病案编目',      icon: 'ri-price-tag-3-line', title: '病案编目管理',     crumb: '首页 / 病案编目' },
    { key: 'borrow',     name: '病案借阅',      icon: 'ri-book-open-line', title: '病案借阅管理',     crumb: '首页 / 病案借阅' },
    { key: 'statistics', name: '数据统计',      icon: 'ri-bar-chart-grouped-line', title: '病案数据统计',     crumb: '首页 / 数据统计' },
    { key: 'report',     name: '首页数据上报',  icon: 'ri-upload-cloud-2-line', title: '病案首页数据上报', crumb: '首页 / 数据上报' },
    { key: 'quality',    name: '质量看板',      icon: 'ri-shield-check-line', title: '病案质量看板',     crumb: '首页 / 质量看板' }
  ];

  /* -------------------- 登录页（route='login'，全屏渲染） -------------------- */
  window.MRPages = window.MRPages || {};
  window.MRPages.Login = {
    name: 'Login',
    setup: function () {
      var S = MRMS.state, A = MRMS.actions;
      var ref = Vue.ref, reactive = Vue.reactive;
      var f = reactive({ username: '', password: '', loading: false });
      async function submit() {
        if (!f.username || !f.password) { A.toast('请输入用户名和密码', 'err'); return; }
        f.loading = true;
        try { await A.login(f.username, f.password); }
        catch (e) { /* toast 已由适配层弹出 */ }
        f.loading = false;
      }
      function quick(u) {
        f.username = u;
        f.password = u === 'admin' ? 'admin123' : '123456';
        submit();
      }
      return { S: S, f: f, submit: submit, quick: quick };
    },
    template: [
      '<div class="login-wrap">',
      '  <div class="login-card">',
      '    <div class="login-brand"><div class="brand-logo" style="width:40px;height:40px;font-size:18px;">病</div>',
      '      <div><strong style="font-size:18px;">病案管理系统</strong><br><span class="login-sub">MRMS · 住院病案归档 / 编目 / 借阅 / 统计 / 上报</span></div></div>',
      '    <div class="login-field"><label>用户名</label>',
      '      <input class="login-input" v-model.trim="f.username" placeholder="请输入用户名" @keyup.enter="submit"></div>',
      '    <div class="login-field"><label>密码</label>',
      '      <input class="login-input" type="password" v-model.trim="f.password" placeholder="请输入密码" @keyup.enter="submit"></div>',
      '    <button class="btn btn-primary login-btn" :disabled="f.loading" @click="submit">{{ f.loading ? "登录中…" : "登 录" }}</button>',
      '    <div class="login-quick">',
      '      <span>快捷登录（角色切换演示）：</span>',
      '      <button class="btn btn-default btn-sm" @click="quick(\'admin\')">病案室管理员</button>',
      '      <button class="btn btn-default btn-sm" @click="quick(\'deptuser\')">科室医师</button>',
      '    </div>',
      '    <div class="login-tip">admin / admin123 · deptuser / 123456</div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  var App = {
    name: 'App',
    setup: function () {
      var S = MRMS.state, A = MRMS.actions;
      var P = MRPages;

      var pageMap = {
        dashboard: P.Dashboard,
        archive: P.Archive,
        catalog: P.Catalog,
        borrow: P.Borrow,
        statistics: P.Statistics,
        report: P.Report,
        quality: P.Quality,
        login: P.Login
      };

      var current = computed(function () { return pageMap[S.route] || P.Dashboard; });
      var meta = computed(function () {
        return NAV.find(function (n) { return n.key === S.route; }) || NAV[0];
      });

      // 侧栏待办角标
      var badges = computed(function () {
        var recs = S.model.records;
        var toCatalog = recs.filter(function (r) { return r.status === '已归档'; }).length;
        var borrowTodo = S.model.borrows.filter(function (b) {
          if (b.status === '待审批') return true;
          if (b.status === '已借出' && MRMS.util.diffDays(b.dueDate, MRMS.util.today()) > 0) return true;
          return false;
        }).length;
        var reportTodo = S.model.reports.filter(function (b) {
          return b.status === '待上报' || b.status === '已退回';
        }).length;
        return { catalog: toCatalog, borrow: borrowTodo, report: reportTodo };
      });

      var todayText = (function () {
        var d = new Date();
        var w = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六'][d.getDay()];
        return d.getFullYear() + ' 年 ' + (d.getMonth() + 1) + ' 月 ' + d.getDate() + ' 日 ' + w;
      })();

      function nav(key) {
        S.route = key;
        S.sidebarOpen = false;
        window.scrollTo({ top: 0, behavior: 'smooth' });
      }

      function logout() {
        A.logout();
        A.toast('已退出登录');
      }

      return {
        S: S, NAV: NAV, current: current, meta: meta, badges: badges,
        todayText: todayText, nav: nav, logout: logout
      };
    },
    template: [
      '<div>',
      '  <component :is="current" v-if="S.route === \'login\'"></component>',
      '  <div class="layout" v-else>',
      /* -------------------- 侧边栏 -------------------- */
      '    <aside class="sidebar" :class="{ open: S.sidebarOpen }">',
      '      <div class="brand">',
      '        <div class="brand-logo">病</div>',
      '        <div class="brand-text">',
      '          <strong>病案管理系统</strong>',
      '          <span>MRMS v1.0</span>',
      '        </div>',
      '      </div>',
      '      <nav class="nav">',
      '        <div class="nav-title">业务功能</div>',
      '        <button v-for="n in NAV" :key="n.key" class="nav-item"',
      '                :class="{ active: S.route === n.key }" @click="nav(n.key)">',
      '          <span class="ico"><i :class="n.icon"></i></span>',
      '          <span>{{ n.name }}</span>',
      '          <span class="badge-dot"',
      '                v-if="n.key === \'catalog\' && badges.catalog">{{ badges.catalog }}</span>',
      '          <span class="badge-dot"',
      '                v-if="n.key === \'borrow\' && badges.borrow">{{ badges.borrow }}</span>',
      '          <span class="badge-dot"',
      '                v-if="n.key === \'report\' && badges.report">{{ badges.report }}</span>',
      '        </button>',
      '      </nav>',
      '      <div class="sidebar-foot">',
      '        <div>当前用户：<b>{{ S.currentUser.name }}</b></div>',
      '        <div>{{ S.currentUser.role }} · {{ S.currentUser.dept }}</div>',
      '      </div>',
      '    </aside>',

      /* -------------------- 主区域 -------------------- */
      '    <div class="main">',
      '      <header class="header">',
      '        <button class="menu-btn" @click="S.sidebarOpen = !S.sidebarOpen"><i class="ri-menu-line"></i></button>',
      '        <div>',
      '          <h1>{{ meta.title }}</h1>',
      '          <div class="crumb">{{ meta.crumb }}</div>',
      '        </div>',
      '        <div class="header-right">',
      '          <span class="header-date hide-sm">{{ todayText }}</span>',
      '          <button class="btn btn-default btn-sm" @click="logout">退出</button>',
      '          <div class="user-chip">',
      '            <div class="avatar">{{ S.currentUser.name.slice(0, 1) }}</div>',
      '            <span>{{ S.currentUser.name }}</span>',
      '          </div>',
      '        </div>',
      '      </header>',
      '      <main class="content">',
      '        <component :is="current"></component>',
      '      </main>',
      '    </div>',
      '  </div>',

      /* -------------------- 全局提示 -------------------- */
      '  <div class="toasts">',
      '    <div class="toast" :class="t.type" v-for="t in S.toasts" :key="t.id">',
      '      <span><i :class="t.type === \'err\' ? \'ri-alert-line\' : \'ri-checkbox-circle-line\'"></i></span><span>{{ t.msg }}</span>',
      '    </div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  window.MRApp = App;
})();
