<template>
  <a
    :href="attachment.url"
    target="_blank"
    rel="noopener noreferrer"
    class="flex items-center gap-3 p-2.5 rounded-md border border-border bg-background hover:bg-muted transition-colors group"
    :title="filename"
  >
    <div class="w-9 h-9 flex-shrink-0 rounded-md bg-muted flex items-center justify-center text-muted-foreground">
      <img
        v-if="isImage && !thumbFailed"
        :src="thumbUrl"
        :alt="filename"
        class="w-full h-full object-cover"
        @error="thumbFailed = true"
      />
      <component v-else :is="fileIcon" :size="20" />
    </div>

    <div class="flex-1 min-w-0">
      <p class="text-sm font-medium text-foreground truncate">{{ filename }}</p>
      <p class="text-xs text-muted-foreground">{{ formatBytes(attachment.size) }}</p>
    </div>

    <Download
      :size="18"
      class="text-muted-foreground flex-shrink-0 opacity-60 group-hover:opacity-100 transition-opacity"
    />
  </a>
</template>

<script setup>
import { computed, ref } from 'vue'
import { formatBytes, getThumbFilepath } from '@shared-ui/utils/file'
import { Download, FileLines as FileText, FileSpreadsheet, FileImage, FileVideo, FileAudio, FileZipper as FileArchive, File } from '@qall/icons'

const props = defineProps({
  attachment: {
    type: Object,
    required: true
  }
})

const filename = computed(() => props.attachment.name || props.attachment.filename || '')

const ext = computed(() => filename.value.toLowerCase().split('.').pop() || '')

const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg']
const isImage = computed(() => {
  if (props.attachment.content_type) {
    return props.attachment.content_type.startsWith('image/')
  }
  return imageExts.includes(ext.value)
})

const thumbFailed = ref(false)
const thumbUrl = computed(() => getThumbFilepath(props.attachment.url))

const fileIcon = computed(() => {
  const map = {
    pdf: FileText,
    doc: FileText, docx: FileText, txt: FileText, rtf: FileText,
    xls: FileSpreadsheet, xlsx: FileSpreadsheet, csv: FileSpreadsheet,
    png: FileImage, jpg: FileImage, jpeg: FileImage, gif: FileImage, webp: FileImage, svg: FileImage,
    mp4: FileVideo, mov: FileVideo, avi: FileVideo, webm: FileVideo,
    mp3: FileAudio, wav: FileAudio, ogg: FileAudio, m4a: FileAudio,
    zip: FileArchive, rar: FileArchive, '7z': FileArchive, tar: FileArchive, gz: FileArchive,
  }
  return map[ext.value] || File
})
</script>
