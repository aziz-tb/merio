import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { adminNavItems } from '@main/constants/navigation'

export function useNavDescription() {
  const route = useRoute()
  const { t } = useI18n()

  return computed(() => {
    const path = route.path
    for (const section of adminNavItems) {
      for (const item of section.children || []) {
        if (item.href === path && item.descriptionKey) {
          return t(item.descriptionKey)
        }
      }
    }
    return ''
  })
}
