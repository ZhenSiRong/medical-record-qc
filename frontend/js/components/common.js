/* ============================================================
   common.js —— 通用 UI 小组件与共享方法
   ============================================================ */
(function () {
  'use strict';

  /* --------------------------- 状态色映射（全局复用） --------------------------- */
  var TONE = {
    // 病案流转
    '未归档': 'orange', '待编目': 'orange', '已归档': 'green', '已编目': 'blue',
    // 上报
    '待上报': 'orange', '已上报': 'green', '未上报': 'gray',
    '已提交': 'blue', '已接收': 'green', '已退回': 'red',
    // 借阅
    '待审批': 'orange', '已借出': 'blue', '已归还': 'green',
    '已逾期': 'red', '已驳回': 'gray',
    // 质量等级
    '甲级': 'green', '乙级': 'cyan', '丙级': 'orange',
    // 风险
    '高': 'red', '中': 'orange', '低': 'gray'
  };

  function mrTone(status) { return TONE[status] || 'gray'; }

  /* ------------------------------ 分页组件 ------------------------------ */
  var MrPager = {
    name: 'MrPager',
    props: {
      total: { type: Number, default: 0 },
      page: { type: Number, default: 1 },
      size: { type: Number, default: 10 }
    },
    emits: ['update:page', 'update:size'],
    computed: {
      pages: function () {
        var max = Math.max(1, Math.ceil(this.total / this.size));
        var cur = Math.min(this.page, max);
        var out = [];
        var start = Math.max(1, cur - 2);
        var end = Math.min(max, start + 4);
        start = Math.max(1, end - 4);
        for (var i = start; i <= end; i++) out.push(i);
        return { list: out, max: max, cur: cur };
      },
      range: function () {
        if (!this.total) return '共 0 条';
        var from = (this.pages.cur - 1) * this.size + 1;
        var to = Math.min(this.total, this.pages.cur * this.size);
        return '第 ' + from + '-' + to + ' 条，共 ' + this.total + ' 条';
      }
    },
    template: [
      '<div class="pager">',
      '  <span class="info">{{ range }}</span>',
      '  <select class="select" style="width:96px;height:28px;margin-left:12px"',
      '          :value="size" @change="$emit(\'update:size\', +$event.target.value)">',
      '    <option :value="10">10 条/页</option>',
      '    <option :value="20">20 条/页</option>',
      '    <option :value="50">50 条/页</option>',
      '  </select>',
      '  <div class="pages">',
      '    <button :disabled="pages.cur <= 1" @click="$emit(\'update:page\', pages.cur - 1)">上一页</button>',
      '    <button v-for="p in pages.list" :key="p" :class="{ on: p === pages.cur }"',
      '            @click="$emit(\'update:page\', p)">{{ p }}</button>',
      '    <button :disabled="pages.cur >= pages.max" @click="$emit(\'update:page\', pages.cur + 1)">下一页</button>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  /* ------------------------------ 空状态 ------------------------------ */
  var MrEmpty = {
    name: 'MrEmpty',
    props: { text: { type: String, default: '暂无数据' }, icon: { type: String, default: 'ri-inbox-2-line' } },
    template: '<div class="empty"><span class="big"><i :class="icon"></i></span>{{ text }}</div>'
  };

  /* ------------------------------ 模态框外壳 ------------------------------ */
  var MrModal = {
    name: 'MrModal',
    props: {
      title: { type: String, default: '' },
      sub: { type: String, default: '' },
      wide: { type: Boolean, default: false }
    },
    emits: ['close'],
    template: [
      '<div class="mask" @click.self="$emit(\'close\')">',
      '  <div class="modal" :class="{ wide: wide }">',
      '    <div class="modal-head">',
      '      <h3>{{ title }}</h3>',
      '      <span class="sub" v-if="sub">{{ sub }}</span>',
      '      <button class="close" @click="$emit(\'close\')"><i class="ri-close-line"></i></button>',
      '    </div>',
      '    <div class="modal-body"><slot></slot></div>',
      '    <div class="modal-foot"><slot name="foot"></slot></div>',
      '  </div>',
      '</div>'
    ].join('\n')
  };

  /* ------------------------------ 进度条 ------------------------------ */
  var MrProgress = {
    name: 'MrProgress',
    props: {
      value: { type: Number, default: 0 },
      tone: { type: String, default: '' }
    },
    template: '<div class="progress" :class="tone"><i :style="{ width: Math.min(100, Math.max(0, value)) + \'%\' }"></i></div>'
  };

  /* --------------------------- 角色权限提示条 ---------------------------
     用途：页面里存在「对当前角色置灰」的写操作时，把原因显式写在页面上。

     为什么需要这个组件（2026-10-09 前端角色门控）：
     用户明确要求控件**不要直接隐藏**——只置灰而不说明，用户看到的是"点了没反应"，
     与"系统坏了"无法区分；直接隐藏则让临床根本不知道系统有这个能力。
     故统一为「置灰 + title 提示 + 本提示条说明原因」，本组件承载第三项，
     并保证同一句话在全站只有一份文案（各页面通过 text 传入具体范围）。
     置灰按钮的 title 文案同样来自适配层能力表（MRMS.util.denyHint），与判据同源。 */
  var MrRoleNotice = {
    name: 'MrRoleNotice',
    props: {
      text: {
        type: String,
        default: '当前账号为临床医师，本页的写操作需病案室权限：相关按钮已置灰，查询与只读查看不受影响。'
      }
    },
    template: [
      '<div class="alert info">',
      '  <span class="ai">i</span>',
      '  <span>{{ text }}</span>',
      '</div>'
    ].join('\n')
  };

  window.MRCommon = {
    MrPager: MrPager,
    MrEmpty: MrEmpty,
    MrModal: MrModal,
    MrProgress: MrProgress,
    MrRoleNotice: MrRoleNotice,
    mrTone: mrTone
  };
})();
