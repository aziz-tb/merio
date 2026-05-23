<!--
  Top-level AI area. Bare router-view — AI children (AgentList, ToolList,
  CreateOrEditAgent, CreateOrEditTool) handle their own chrome internally
  (flex flex-col h-full + per-section padding + internal overflow-auto),
  matching the Knowledge area's pattern. No shared help rail.
-->
<template>
  <router-view />
</template>
