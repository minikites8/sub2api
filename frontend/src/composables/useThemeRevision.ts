import { onBeforeUnmount, onMounted, ref } from 'vue'

export function useThemeRevision() {
  const revision = ref(0)
  let observer: MutationObserver | null = null

  onMounted(() => {
    observer = new MutationObserver(() => {
      revision.value += 1
    })

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class']
    })
  })

  onBeforeUnmount(() => {
    observer?.disconnect()
    observer = null
  })

  return revision
}
