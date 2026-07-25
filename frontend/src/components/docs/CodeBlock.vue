<template>
  <div class="docs-code-block">
    <div class="docs-code-toolbar">
      <span>{{ language }}</span>
      <button type="button" :aria-label="copyLabel" :title="copyLabel" @click="emit('copy')">
        <Icon :name="copied ? 'check' : 'copy'" size="sm" />
        <span>{{ copyLabel }}</span>
      </button>
    </div>
    <pre><code>{{ code }}</code></pre>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  language: string
  code: string
  copyLabel: string
  copied: boolean
}>()

const emit = defineEmits<{
  copy: []
}>()
</script>

<style scoped>
.docs-code-block {
  overflow: hidden;
  border: 1px solid #3b4a3f;
  border-radius: 6px;
  background: #010409;
  transition: border-color 160ms ease;
}

.docs-code-block:hover {
  border-color: #00e38b;
}

.docs-code-toolbar {
  display: flex;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid #3b4a3f;
  padding: 7px 12px 7px 16px;
  color: #b9cbbc;
  background: #060f16;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
}

.docs-code-toolbar > span {
  text-transform: lowercase;
}

.docs-code-toolbar button {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 7px;
  border: 1px solid transparent;
  border-radius: 4px;
  padding: 4px 7px;
  color: #b9cbbc;
  background: transparent;
  font: inherit;
  cursor: pointer;
}

.docs-code-toolbar button:hover,
.docs-code-toolbar button:focus-visible {
  border-color: #3b4a3f;
  color: #00ff9d;
  outline: none;
}

pre {
  margin: 0;
  overflow-x: auto;
  padding: 20px 18px;
}

code {
  color: #dae3ee;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  font-weight: 450;
  line-height: 1.75;
  white-space: pre;
}

@media (max-width: 600px) {
  pre {
    padding: 16px 14px;
  }

  code {
    font-size: 12px;
    line-height: 1.7;
  }
}
</style>
