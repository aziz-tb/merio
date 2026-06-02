<template>
  <ContactList>
    <PageHeader />
    <ContactsList />
  </ContactList>
</template>
<script setup>
import PageHeader from '@main/components/layout/PageHeader.vue'
import ContactsList from '@/features/contact/ContactsList.vue'
import ContactList from '@/layouts/contact/ContactList.vue'
</script>
