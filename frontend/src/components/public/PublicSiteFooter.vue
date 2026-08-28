<template>
  <footer class="public-site-footer" :class="`public-site-footer--${theme}`">
    <span>&copy; {{ currentYear }} {{ siteName }} - {{ description }}</span>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/stores'

withDefaults(defineProps<{ description: string; theme?: 'home' | 'docs' | 'pricing' | 'models' }>(), {
  theme: 'home',
})

const appStore = useAppStore()
const currentYear = new Date().getFullYear()
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
</script>

<style scoped>
.public-site-footer {
  width: 100%;
  box-sizing: border-box;
  display: flex;
  justify-content: center;
  padding: 24px 20px;
  border-top: 1px solid rgb(55 65 81);
  color: rgb(107 114 128);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}

.public-site-footer--models {
  max-width: 1440px;
  margin: 8px auto 0;
  padding-inline: 0;
}
</style>
