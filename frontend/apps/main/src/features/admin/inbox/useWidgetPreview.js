import { watch, onBeforeUnmount } from 'vue'

const SCRIPT_ID = 'merio-widget-preview-loader'

// Loads the production widget.js loader onto the current page (once) and resolves
// when window.Merio is available. We never set window.MerioSettings, so the
// loader's auto-init is a no-op and we control the lifecycle manually.
function loadLoader (baseUrl) {
  return new Promise((resolve, reject) => {
    if (window.Merio) return resolve()
    const existing = document.getElementById(SCRIPT_ID)
    if (existing) {
      existing.addEventListener('load', () => resolve(), { once: true })
      existing.addEventListener('error', reject, { once: true })
      return
    }
    const script = document.createElement('script')
    script.id = SCRIPT_ID
    script.src = `${baseUrl}/widget.js`
    script.async = true
    script.addEventListener('load', () => resolve(), { once: true })
    script.addEventListener('error', reject, { once: true })
    document.head.appendChild(script)
  })
}

/**
 * Renders the real chat widget over the current page as a live preview, exactly
 * as it appears embedded on a customer site. Unsaved edits are pushed via the
 * loader's updateConfig() so the launcher and panel update in place.
 *
 * @param {object} opts
 * @param {import('vue').Ref<boolean>} opts.enabled  Toggle on/off.
 * @param {{value: string}} opts.baseUrl             App root URL (getter/ref).
 * @param {{value: string}} opts.inboxUuid           Inbox UUID (getter/ref).
 * @param {{value: object}} opts.config              Live widget config snapshot.
 */
export function useWidgetPreview ({ enabled, baseUrl, inboxUuid, config }) {
  let instance = null
  let debounce = null

  const pushConfig = () => {
    if (instance?.updateConfig && config.value) {
      // JSON round-trip strips Vue reactivity proxies from the payload.
      instance.updateConfig(JSON.parse(JSON.stringify(config.value)))
    }
  }

  // The widget panel posts VUE_APP_READY once it has mounted; that's the safe
  // moment to push unsaved edits (the launcher is already built by then too).
  const onMessage = (e) => {
    if (e?.data?.type === 'VUE_APP_READY' || e?.data?.type === 'WIDGET_LOADED') {
      pushConfig()
    }
  }

  const start = async () => {
    if (instance || !baseUrl.value || !inboxUuid.value) return
    await loadLoader(baseUrl.value)
    // window.Merio is the loader class on first use, or a prior instance after a
    // previous preview session; either way recover the constructor and bypass the
    // loader's single-instance guard so re-enabling works.
    const MerioClass = typeof window.Merio === 'function' ? window.Merio : window.Merio?.constructor
    if (!MerioClass) return
    window.addEventListener('message', onMessage)
    instance = new MerioClass({ baseURL: baseUrl.value, inboxID: inboxUuid.value })
    window.Merio = instance
  }

  const stop = () => {
    clearTimeout(debounce)
    window.removeEventListener('message', onMessage)
    if (instance?.destroy) instance.destroy()
    instance = null
  }

  watch(enabled, (on) => { on ? start() : stop() }, { immediate: true })

  watch(
    config,
    () => {
      if (!instance) return
      clearTimeout(debounce)
      debounce = setTimeout(pushConfig, 200)
    },
    { deep: true }
  )

  onBeforeUnmount(stop)
}
