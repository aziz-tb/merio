<script setup>
import { cn } from '../../../lib/utils'

const props = defineProps({
  class: { type: null, required: false },
})
</script>

<template>
  <li :class="cn('group/menu-item relative', props.class)">
    <slot />
  </li>
</template>
