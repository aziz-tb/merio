<script>
import { computed, defineComponent, h } from 'vue'
import { useRoute } from 'vue-router'
import AIAgentEditContextProvider from '@main/components/layout/providers/AIAgentEditContextProvider.vue'

// Route-scoped provider registry. Providers mounted here sit above both
// the global secondary sidebar and the active route content, so any page
// can share state between those two siblings by opting into a
// `meta.layoutContext` key in the router config.
const providerRegistry = {
  'ai-agent-edit': AIAgentEditContextProvider,
}

export default defineComponent({
  name: 'RouteContextHost',
  setup(_, { slots }) {
    const route = useRoute()

    const providers = computed(() => {
      const names = route.matched.flatMap((record) => {
        const key = record.meta?.layoutContext
        if (!key) return []
        return Array.isArray(key) ? key : [key]
      })

      return [...new Set(names)]
        .map((name) => providerRegistry[name])
        .filter(Boolean)
    })

    return () =>
      providers.value.reduceRight(
        (children, ProviderComponent) =>
          h(ProviderComponent, null, {
            default: () => children,
          }),
        slots.default ? slots.default() : []
      )
  },
})
</script>
