<template>
  <div class="google-adsense-ad">
    <ins
      ref="adRef"
      class="adsbygoogle"
      style="display: block"
      :data-ad-client="client"
      :data-ad-slot="adSlot"
      :data-ad-format="format"
      :data-full-width-responsive="fullWidthResponsive"
    ></ins>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'

type AdSenseQueue = Array<Record<string, unknown>>

declare global {
  interface Window {
    adsbygoogle?: AdSenseQueue
  }
}

const ADSENSE_SCRIPT_ID = 'google-adsense-script'

const props = withDefaults(
  defineProps<{
    client: string
    adSlot: string
    format?: string
    fullWidthResponsive?: 'true' | 'false'
  }>(),
  {
    format: 'auto',
    fullWidthResponsive: 'true'
  }
)

const adRef = ref<HTMLElement | null>(null)

const getCSPNonce = () => document.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || ''

const loadAdSenseScript = () => {
  if (document.getElementById(ADSENSE_SCRIPT_ID)) {
    return
  }

  const script = document.createElement('script')
  script.id = ADSENSE_SCRIPT_ID
  script.async = true
  const nonce = getCSPNonce()
  if (nonce) {
    script.nonce = nonce
  }
  script.src = `https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=${encodeURIComponent(props.client)}`
  script.crossOrigin = 'anonymous'
  document.head.appendChild(script)
}

const requestAd = () => {
  if (!adRef.value) {
    return
  }

  try {
    window.adsbygoogle = window.adsbygoogle || []
    window.adsbygoogle.push({})
  } catch (error) {
    console.warn('Failed to initialize AdSense ad:', error)
  }
}

onMounted(async () => {
  loadAdSenseScript()
  await nextTick()
  requestAd()
})
</script>

<style scoped>
.google-adsense-ad {
  width: 100%;
  overflow: hidden;
}
</style>
