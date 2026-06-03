<template>
  <ContactList>
    <PageHeader class="shrink-0" />
    <ContactsList class="min-h-0 flex-1" />
  </ContactList>
</template>
<script setup>
import PageHeader from '@main/components/layout/PageHeader.vue'
import ContactsList from '@/features/contact/ContactsList.vue'
import ContactList from '@/layouts/contact/ContactList.vue'
</script>
