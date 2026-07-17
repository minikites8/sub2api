<template>
  <div class="table-page-layout" :class="{ 'mobile-mode': isMobile }">
    <!-- 固定区域：操作按钮 -->
    <div
      v-if="$slots.actions"
      class="layout-section layout-section-fixed layout-section-actions"
      data-layout-section="actions"
    >
      <div class="layout-section-content">
        <slot name="actions" />
      </div>
    </div>

    <!-- 固定区域：搜索和过滤器 -->
    <div
      v-if="$slots.filters"
      class="layout-section layout-section-fixed layout-section-filters"
      data-layout-section="filters"
    >
      <div class="layout-section-content">
        <slot name="filters" />
      </div>
    </div>

    <!-- 滚动区域：表格 -->
    <div class="layout-section-scrollable">
      <div class="card table-scroll-container">
        <slot name="table" />
      </div>
    </div>

    <!-- 固定区域：分页器 -->
    <div
      v-if="$slots.pagination"
      class="layout-section layout-section-fixed layout-section-pagination"
      data-layout-section="pagination"
    >
      <div class="layout-section-content">
        <slot name="pagination" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const isMobile = ref(false)

const checkMobile = () => {
  isMobile.value = window.innerWidth < 1024
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
/* 桌面端：Flexbox 布局 */
.table-page-layout {
  @apply flex flex-col gap-5;
  height: calc(100vh - 56px - 3rem);
}

.layout-section {
  @apply flex-shrink-0;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface-container-low);
  color: var(--md-on-surface);
  padding: 12px;
  box-shadow: none;
}

.layout-section-content {
  width: 100%;
}

.layout-section-actions {
  border-color: transparent;
  background: transparent;
  padding: 0;
}

.layout-section-filters,
.layout-section-pagination {
  background: var(--md-surface-container-low);
}

.layout-section-scrollable {
  @apply flex-1 min-h-0 flex flex-col;
}

.layout-section-fixed :deep([class*="bg-primary-"]),
.layout-section-fixed :deep([class*="bg-blue-"]),
.layout-section-fixed :deep([class*="bg-sky-"]),
.layout-section-fixed :deep([class*="bg-cyan-"]),
.layout-section-fixed :deep([class*="bg-indigo-"]),
.layout-section-fixed :deep([class*="bg-violet-"]),
.layout-section-fixed :deep([class*="bg-purple-"]) {
  background-color: var(--md-surface-container) !important;
}

.layout-section-fixed :deep([class*="bg-gradient-to-"]),
.layout-section-fixed :deep([class*="from-primary-"]),
.layout-section-fixed :deep([class*="from-blue-"]),
.layout-section-fixed :deep([class*="from-sky-"]),
.layout-section-fixed :deep([class*="from-cyan-"]),
.layout-section-fixed :deep([class*="from-indigo-"]),
.layout-section-fixed :deep([class*="from-violet-"]),
.layout-section-fixed :deep([class*="from-purple-"]),
.layout-section-fixed :deep([class*="via-primary-"]),
.layout-section-fixed :deep([class*="via-blue-"]),
.layout-section-fixed :deep([class*="via-sky-"]),
.layout-section-fixed :deep([class*="via-cyan-"]),
.layout-section-fixed :deep([class*="via-indigo-"]),
.layout-section-fixed :deep([class*="via-violet-"]),
.layout-section-fixed :deep([class*="via-purple-"]),
.layout-section-fixed :deep([class*="to-primary-"]),
.layout-section-fixed :deep([class*="to-blue-"]),
.layout-section-fixed :deep([class*="to-sky-"]),
.layout-section-fixed :deep([class*="to-cyan-"]),
.layout-section-fixed :deep([class*="to-indigo-"]),
.layout-section-fixed :deep([class*="to-violet-"]),
.layout-section-fixed :deep([class*="to-purple-"]) {
  background-color: var(--md-surface-container) !important;
  background-image: none !important;
}

.layout-section-fixed :deep([class*="text-primary-"]),
.layout-section-fixed :deep([class*="text-blue-"]),
.layout-section-fixed :deep([class*="text-sky-"]),
.layout-section-fixed :deep([class*="text-cyan-"]),
.layout-section-fixed :deep([class*="text-indigo-"]),
.layout-section-fixed :deep([class*="text-violet-"]),
.layout-section-fixed :deep([class*="text-purple-"]) {
  color: var(--md-on-surface) !important;
}

.layout-section-fixed :deep([class*="border-primary-"]),
.layout-section-fixed :deep([class*="border-blue-"]),
.layout-section-fixed :deep([class*="border-sky-"]),
.layout-section-fixed :deep([class*="border-cyan-"]),
.layout-section-fixed :deep([class*="border-indigo-"]),
.layout-section-fixed :deep([class*="border-violet-"]),
.layout-section-fixed :deep([class*="border-purple-"]) {
  border-color: var(--md-outline-variant) !important;
}

.layout-section-fixed :deep([class*="ring-primary-"]),
.layout-section-fixed :deep([class*="ring-blue-"]),
.layout-section-fixed :deep([class*="ring-sky-"]),
.layout-section-fixed :deep([class*="ring-cyan-"]),
.layout-section-fixed :deep([class*="ring-indigo-"]),
.layout-section-fixed :deep([class*="ring-violet-"]),
.layout-section-fixed :deep([class*="ring-purple-"]) {
  --tw-ring-color: var(--md-state-focus) !important;
}

.layout-section-fixed :deep(.card) {
  border-color: var(--md-outline-variant);
  background: var(--md-surface) !important;
  box-shadow: none;
}

.layout-section-filters :deep(> .layout-section-content > .card),
.layout-section-pagination :deep(> .layout-section-content > .card) {
  border: 0;
  background: transparent !important;
  box-shadow: none;
}

.layout-section-actions :deep(.card) {
  background: var(--md-surface) !important;
}

/* 表格滚动容器 - 增强版表体滚动方案 */
.table-scroll-container {
  @apply flex h-full flex-col overflow-hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.table-scroll-container :deep(.table-wrapper) {
  @apply flex-1 overflow-x-auto overflow-y-auto;
  /* 确保横向滚动条显示在最底部 */
  scrollbar-gutter: stable;
}

.table-scroll-container :deep(table) {
  @apply w-full;
  min-width: max-content; /* 关键：确保表格宽度根据内容撑开，从而触发横向滚动 */
  display: table; /* 使用标准 table 布局以支持 sticky 列 */
}

.table-scroll-container :deep(thead) {
  background: var(--md-surface-container-low);
}

.table-scroll-container :deep(tbody) {
  /* 保持默认 table-row-group 显示，不使用 block */
  background: var(--md-surface);
}

.table-scroll-container :deep(tbody tr),
.table-scroll-container :deep(tbody td) {
  background: var(--md-surface);
  transition: background-color 0.15s ease;
}

.table-scroll-container :deep(tbody tr:hover td) {
  background: var(--md-surface-container-low);
}

.table-scroll-container :deep(th) {
  @apply px-5 py-4 text-left text-sm font-medium;
  border-bottom: 1px solid var(--md-outline-variant);
  color: var(--md-on-surface-variant);
}

.table-scroll-container :deep(td) {
  @apply px-5 py-4 text-sm;
  border-bottom: 1px solid var(--md-outline-variant);
  color: var(--md-on-surface);
}

.table-scroll-container :deep(tbody tr:last-child td) {
  border-bottom: 0;
}

.table-scroll-container :deep(tbody .sticky-col) {
  background: var(--md-surface);
}

.table-scroll-container :deep(tbody tr:hover .sticky-col) {
  background: var(--md-surface-container-low);
}

/* 移动端：恢复正常滚动 */
.table-page-layout.mobile-mode .table-scroll-container {
  @apply h-auto overflow-visible border-none shadow-none bg-transparent;
}

.table-page-layout.mobile-mode .layout-section-fixed {
  border-radius: 10px;
  padding: 10px;
}

.table-page-layout.mobile-mode .layout-section-scrollable {
  @apply flex-none min-h-fit;
}

.table-page-layout.mobile-mode .table-scroll-container :deep(table) {
  @apply flex-none;
  display: table;
  min-width: 100%;
}
</style>
