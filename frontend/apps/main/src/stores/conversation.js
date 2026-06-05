import { defineStore } from 'pinia'
import { computed, reactive, ref, watchEffect } from 'vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { TYPING_RECEIVE_TIMEOUT } from '@shared-ui/composables/useTypingIndicator.js'
import { deepMerge } from '@shared-ui/utils/object.js'
import { computeRecipientsFromMessage } from '../utils/email-recipients'
import { useEmitter } from '../composables/useEmitter'
import { EMITTER_EVENTS } from '../constants/emitterEvents'
import { INBOX_CHANNEL } from '../constants/inbox'
import { subscribeToConversation, sendTypingIndicator } from '@main/websocket'
import { playNotificationSound } from '@shared-ui/composables/useNotificationSound'
import MessageCache from '../utils/conversation-message-cache'
import { getI18n } from '../i18n'
import { useThrottleFn } from '@vueuse/core'
import { CONVERSATION_LIST_TYPE, CONVERSATION_DEFAULT_STATUSES } from '@/constants/conversation'
import { getPriorityVisual } from '@/utils/priorityVisuals'
import { getStatusVisual } from '@/utils/statusVisuals'
import { useInboxStore } from './inbox'
import api from '../api'

export const useConversationStore = defineStore('conversation', () => {
  const CONV_LIST_PAGE_SIZE = 25
  const MESSAGE_LIST_PAGE_SIZE = 30
  const priorities = ref([])
  const statuses = ref([])
  const currentTo = ref([])
  const currentBCC = ref([])
  const currentCC = ref([])
  const macros = ref({})
  const drafts = ref(new Map())

  // Per-conversation viewers (other agents currently subscribed via WebSocket).
  // Keyed by conversation UUID → array of user IDs (deduped server-side).
  // Each `conversation_viewers` broadcast is authoritative; setViewers replaces the entry verbatim.
  const viewers = ref({})
  const setViewers = (conversationUUID, userIds) => {
    if (!conversationUUID) return
    viewers.value[conversationUUID] = Array.isArray(userIds) ? userIds : []
  }

  // Options for select fields
  const priorityOptions = computed(() => {
    return priorities.value.map(p => {
      const visual = getPriorityVisual(p.name)
      return {
        label: p.name,
        value: p.id,
        icon: visual.icon
      }
    })
  })
  const statusOptions = computed(() => {
    return statuses.value.map(s => ({
      label: s.name,
      value: s.id,
      category: s.category,
      ...getStatusVisual(s)
    }))
  })
  // Status options excluding 'Snoozed'
  const statusOptionsNoSnooze = computed(() =>
    statuses.value.filter(s => s.name !== 'Snoozed').map(s => ({
      label: s.name,
      value: s.id,
      category: s.category,
      ...getStatusVisual(s)
    }))
  )

  // TODO: Move to constants.
  const sortFieldMap = {
    oldest: {
      model: 'conversations',
      field: 'last_message_at',
      order: 'asc'
    },
    newest: {
      model: 'conversations',
      field: 'last_message_at',
      order: 'desc'
    },
    started_first: {
      model: 'conversations',
      field: 'created_at',
      order: 'asc'
    },
    started_last: {
      model: 'conversations',
      field: 'created_at',
      order: 'desc'
    },
    waiting_longest: {
      model: 'conversations',
      field: 'waiting_since',
      order: 'asc'
    },
    next_sla_target: {
      model: 'conversations',
      field: 'next_sla_deadline_at',
      order: 'asc'
    },
    priority_first: {
      model: 'conversations',
      field: 'priority_id',
      order: 'desc'
    }
  }

  const sortFieldI18nKeys = {
    oldest: 'conversation.sort.oldestActivity',
    newest: 'conversation.sort.newestActivity',
    started_first: 'conversation.sort.startedFirst',
    started_last: 'conversation.sort.startedLast',
    waiting_longest: 'conversation.sort.waitingLongest',
    next_sla_target: 'conversation.sort.nextSLATarget',
    priority_first: 'conversation.sort.priorityFirst'
  }

  let typingTimeout = null

  const conversations = reactive({
    data: [],
    listType: null,
    status: 'Open',
    sortField: 'newest',
    listFilters: [],
    viewID: 0,
    teamID: 0,
    loading: false,
    page: 1,
    hasMore: false,
    total: 0,
    errorMessage: ''
  })

  const stats = reactive({
    lists: {},
    teams: {},
    views: {}
  })

  const conversation = reactive({
    data: null,
    loading: false,
    errorMessage: '',
    isTyping: false
  })

  const messages = reactive({
    data: new MessageCache(),
    loading: false,
    page: 1,
    // To trigger reactivity on the messages cache, simpler than making MessageCache reactive.
    version: 0,
  })

  let seenConversationUUIDs = new Map()
  // Bumped on resetConversations() so in-flight list requests can drop stale responses.
  let contextSeq = 0
  const emitter = useEmitter()

  const incrementMessageVersion = () => setTimeout(() => messages.version++, 0)

  function setListStatus (status, fetch = true) {
    conversations.status = status
    if (fetch) {
      resetConversations()
      reFetchConversationsList()
    }
  }

  const getListStatus = computed(() => {
    return conversations.status
  })

  function setListSortField (field) {
    if (conversations.sortField === field) return
    conversations.sortField = field
    resetConversations()
    reFetchConversationsList()
  }

  const getListSortField = computed(() => {
    const i18n = getI18n()
    const t = i18n?.global?.t || ((key) => key.split('.').pop())
    return t(sortFieldI18nKeys[conversations.sortField])
  })


  async function fetchStatuses () {
    if (statuses.value.length > 0) return
    try {
      const response = await api.getStatuses()
      statuses.value = response.data.data.map(status => ({
        ...status,
        id: status.id.toString()
      }))
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function fetchPriorities () {
    if (priorities.value.length > 0) return
    try {
      const response = await api.getPriorities()
      priorities.value = response.data.data.map(priority => ({
        ...priority,
        id: priority.id.toString()
      }))
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function fetchConversationStats () {
    try {
      const response = await api.getConversationStats()
      const nextStats = response.data?.data || {}
      stats.lists = nextStats.lists || {}
      stats.teams = nextStats.teams || {}
      stats.views = nextStats.views || {}
    } catch (error) {
      console.error('Error fetching conversation stats:', handleHTTPError(error).message)
    }
  }

  const conversationsList = computed(() => {
    if (!conversations.data) return []
    let filteredConversations = conversations.data
    // Filter by status if set.
    if (conversations.status !== "") {
      filteredConversations = conversations.data
        .filter(conv => {
          return conv.status === conversations.status
        })
    }

    // Sort conversations based on the selected sort field
    return [...filteredConversations].sort((a, b) => {
      const field = sortFieldMap[conversations.sortField]?.field
      if (!a[field] && !b[field]) return 0
      if (!a[field]) return 1       // null goes last
      if (!b[field]) return -1
      const order = sortFieldMap[conversations.sortField]?.order
      return order === 'asc'
        ? new Date(a[field]) - new Date(b[field])
        : new Date(b[field]) - new Date(a[field])
    })
  })

  const currentConversationHasMoreMessages = computed(() => {
    return messages.data.hasMore(conversation.data?.uuid)
  })

  const conversationMessages = computed(() => {
    return messages.data.getAllPagesMessages(conversation.data?.uuid)
  })

  function markConversationAsRead (uuid) {
    const index = conversations.data.findIndex(conv => conv.uuid === uuid)
    if (index !== -1) {
      setTimeout(() => {
        if (conversations.data?.[index]) {
          conversations.data[index].unread_message_count = 0
        }
      }, 3000)
    }
  }

  async function markAsUnread (uuid) {
    try {
      await api.markConversationAsUnread(uuid)
      const index = conversations.data.findIndex(conv => conv.uuid === uuid)
      if (index !== -1) {
        conversations.data[index].unread_message_count = 1
      }
    } catch (err) {
      handleHTTPError(err)
    }
  }

  const currentContactName = computed(() => {
    if (!conversation.data?.contact) return ''
    return conversation.data?.contact.first_name + ' ' + conversation.data?.contact.last_name
  })

  function getContactFullName (uuid) {
    if (conversations?.data) {
      const conv = conversations.data.find(conv => conv.uuid === uuid)
      return conv ? `${conv.contact.first_name} ${conv.contact.last_name}` : ''
    }
  }

  const current = computed(() => {
    return conversation.data || {}
  })

  const isConversationOpen = computed(() => {
    return Object.keys(conversation.data || {}).length > 0
  })

  // Watch for changes in the conversation and messages and update the to, cc, and bcc
  watchEffect(async () => {
    const _ = messages.version // eslint-disable-line no-unused-vars
    const conv = conversation.data
    const msgData = messages.data
    const inboxEmail = conv?.inbox_mail

    // If the conversation is a live chat, reset recipients.
    if (conv?.inbox_channel === 'livechat') {
      currentTo.value = []
      currentCC.value = []
      currentBCC.value = []
      return
    }

    if (!conv || !msgData || !inboxEmail) return

    const latestMessage = msgData.getLatestMessage(conv.uuid, ['incoming', 'outgoing'], true, { excludeAutomated: true })
    if (!latestMessage) {
      // Reset recipients if no latest message is found.
      currentTo.value = []
      currentCC.value = []
      currentBCC.value = []
      return
    }

    const { to, cc, bcc } = computeRecipientsFromMessage(
      latestMessage,
      conv.contact?.email || '',
      inboxEmail,
      conv?.inbox_reply_to || ''
    )
    currentTo.value = to
    currentCC.value = cc
    currentBCC.value = bcc
  })

  function recomputeRecipientsForInbox (inboxId) {
    const conv = conversation.data
    if (!conv) return

    const inboxStore = useInboxStore()
    const inbox = inboxStore.inboxes.find((i) => Number(i.id) === Number(inboxId))
    if (!inbox || inbox.channel !== INBOX_CHANNEL.EMAIL) return

    const latestMessage = messages.data.getLatestMessage(conv.uuid, ['incoming', 'outgoing'], true, { excludeAutomated: true })
    if (!latestMessage) {
      currentTo.value = []
      currentCC.value = []
      currentBCC.value = []
      return
    }

    const { to, cc, bcc } = computeRecipientsFromMessage(
      latestMessage,
      conv.contact?.email || '',
      inbox.from || '',
      inbox.reply_to || inbox.config?.reply_to || ''
    )
    currentTo.value = to
    currentCC.value = cc
    currentBCC.value = bcc
  }

  async function fetchConversation (uuid) {
    conversation.loading = true
    try {
      const resp = await api.getConversation(uuid)
      conversation.data = resp.data.data
      conversation.isTyping = false
      if (typingTimeout) {
        clearTimeout(typingTimeout)
        typingTimeout = null
      }
      // Do a websocket subscription to the conversation.
      subscribeToConversation(uuid)
    } catch (error) {
      conversation.errorMessage = handleHTTPError(error).message
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: conversation.errorMessage
      })
    } finally {
      conversation.loading = false
    }
  }

  /**
   * Fetches messages for a conversation if not already present in the cache.
   * 
   * @param {string} uuid
   * @returns 
   */
  async function fetchMessages (uuid, fetchNextPage = false) {
    // Messages are already cached?
    let hasMessages = messages.data.getAllPagesMessages(uuid)
    if (hasMessages.length > 0 && !fetchNextPage) {
      markConversationAsRead(uuid)
      return
    }

    // Fetch messages from server.
    messages.loading = true
    // Increment page number
    let page = messages.data.getLastFetchedPage(uuid) + 1
    try {
      const response = await api.getConversationMessages(uuid, { page: page, page_size: MESSAGE_LIST_PAGE_SIZE })
      const result = response.data?.data || {}
      const newMessages = result.results || []
      markConversationAsRead(uuid)
      // Cache messages
      messages.data.addMessages(uuid, newMessages, result.page, result.total_pages)
      incrementMessageVersion()
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    } finally {
      messages.loading = false
    }
  }

  async function fetchNextMessages () {
    fetchMessages(conversation.data.uuid, true)
  }

  /**
   * Fetches a single message from the server and adds it to the message cache.
   * 
   * @param {string} conversationUUID
   * @param {string} messageUUID
   * @returns {object}
   */
  async function fetchMessage (conversationUUID, messageUUID) {
    try {
      const response = await api.getConversationMessage(conversationUUID, messageUUID)
      if (response?.data?.data) {
        const newMsg = response.data.data
        // Add message to cache.
        messages.data.addMessage(conversationUUID, newMsg)
        incrementMessageVersion()
        return newMsg
      }
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  function fetchNextConversations () {
    if (conversations.loading || !conversations.hasMore) return
    fetchConversationsList(true, conversations.listType, conversations.teamID, conversations.listFilters, conversations.viewID, conversations.page + 1)
  }

  function reFetchConversationsList (showLoader = true) {
    fetchConversationsList(showLoader, conversations.listType, conversations.teamID, conversations.listFilters, conversations.viewID, conversations.page)
  }

  async function fetchFirstPageConversations () {
    await fetchConversationsList(false, conversations.listType, conversations.teamID, conversations.listFilters, conversations.viewID, 1)
  }

  async function fetchConversationsList (showLoader = true, listType = null, teamID = 0, filters = [], viewID = 0, page = 0) {
    if (!listType) return
    if (conversations.listType !== listType || conversations.teamID !== teamID || conversations.viewID !== viewID) {
      resetConversations()
    }
    if (listType) conversations.listType = listType
    if (teamID) conversations.teamID = teamID
    if (viewID) conversations.viewID = viewID
    if (conversations.status) {
      filters = filters.filter(f => f.model !== 'conversation_statuses')
      filters.push({
        model: 'conversation_statuses',
        field: 'name',
        operator: 'equals',
        value: conversations.status
      })
    }
    if (filters) conversations.listFilters = filters
    if (showLoader) conversations.loading = true
    if (page === 0)
      page = conversations.page
    const seq = contextSeq
    try {
      conversations.errorMessage = ''
      const response = await makeConversationListRequest(listType, teamID, viewID, filters, page)
      // Drop response if the list context (type/team/view) switched mid-flight.
      if (seq !== contextSeq) return
      processConversationListResponse(response)
    } catch (error) {
      if (seq !== contextSeq) return
      // Keep the existing list visible on error; only surface the error on an empty list.
      if (conversations.data.length === 0) {
        conversations.errorMessage = handleHTTPError(error).message
        conversations.total = 0
      }
    } finally {
      if (seq === contextSeq) conversations.loading = false
    }
  }

  async function makeConversationListRequest (listType, teamID, viewID, filters, page) {
    filters = filters.length > 0 ? JSON.stringify(filters) : []
    switch (listType) {
      case CONVERSATION_LIST_TYPE.ASSIGNED:
        return await api.getAssignedConversations({
          page: page,
          page_size: CONV_LIST_PAGE_SIZE,
          order_by: sortFieldMap[conversations.sortField].model + "." + sortFieldMap[conversations.sortField].field,
          order: sortFieldMap[conversations.sortField].order,
          filters
        })
      case CONVERSATION_LIST_TYPE.UNASSIGNED:
        return await api.getUnassignedConversations({
          page: page,
          page_size: CONV_LIST_PAGE_SIZE,
          order_by: sortFieldMap[conversations.sortField].model + "." + sortFieldMap[conversations.sortField].field,
          order: sortFieldMap[conversations.sortField].order,
          filters
        })
      case CONVERSATION_LIST_TYPE.ALL:
        return await api.getAllConversations({
          page: page,
          page_size: CONV_LIST_PAGE_SIZE,
          order_by: sortFieldMap[conversations.sortField].model + "." + sortFieldMap[conversations.sortField].field,
          order: sortFieldMap[conversations.sortField].order,
          filters
        })
      case CONVERSATION_LIST_TYPE.TEAM_UNASSIGNED:
        return await api.getTeamUnassignedConversations(teamID, {
          page: page,
          page_size: CONV_LIST_PAGE_SIZE,
          order_by: sortFieldMap[conversations.sortField].model + "." + sortFieldMap[conversations.sortField].field,
          order: sortFieldMap[conversations.sortField].order,
          filters
        })
      case CONVERSATION_LIST_TYPE.VIEW:
        return await api.getViewConversations(viewID, {
          page: page,
          page_size: CONV_LIST_PAGE_SIZE,
          order_by: sortFieldMap[conversations.sortField].model + "." + sortFieldMap[conversations.sortField].field,
          order: sortFieldMap[conversations.sortField].order
        })
      case CONVERSATION_LIST_TYPE.MENTIONED:
        return await api.getMentionedConversations({
          page: page,
          page_size: CONV_LIST_PAGE_SIZE,
          order_by: sortFieldMap[conversations.sortField].model + "." + sortFieldMap[conversations.sortField].field,
          order: sortFieldMap[conversations.sortField].order,
          filters
        })
      default:
        throw new Error('Invalid conversation list type: ' + listType)
    }
  }

  function processConversationListResponse (response) {
    const apiResponse = response.data.data
    const newConversations = []
    for (const conv of apiResponse.results) {
      if (seenConversationUUIDs.has(conv.uuid)) {
        // Update existing conversation with fresh data.
        const idx = conversations.data.findIndex(c => c.uuid === conv.uuid)
        if (idx !== -1) {
          deepMerge(conversations.data[idx], conv)
        }
      } else {
        // Add to seen and new conversations list.
        seenConversationUUIDs.set(conv.uuid, true)
        newConversations.push(conv)
      }
    }
    // Advance the page only on a successful response, never optimistically.
    conversations.page = Math.max(conversations.page, apiResponse.page)
    conversations.hasMore = apiResponse.total_pages > conversations.page
    if (!conversations.data) conversations.data = []
    conversations.data.push(...newConversations)
    conversations.total = apiResponse.total

    // Play notification sound for new conversations that were pending.
    if (pendingNotificationUUIDs.size > 0) {
      let shouldPlay = false
      for (const uuid of pendingNotificationUUIDs) {
        if (isConversationInList(uuid)) {
          shouldPlay = true
        }
      }
      pendingNotificationUUIDs.clear()
      if (shouldPlay) {
        playNotificationSound()
      }
    }
  }

  async function updatePriority (v, uuid = conversation.data?.uuid) {
    if (!uuid) return
    try {
      await api.updateConversationPriority(uuid, { priority: v })
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function updateStatus (v, uuid = conversation.data?.uuid) {
    if (!uuid) return
    try {
      await api.updateConversationStatus(uuid, { status: v })
      fetchConversationStats()
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function snoozeConversation (snoozeDuration, uuid = conversation.data?.uuid) {
    if (!uuid) return
    try {
      await api.updateConversationStatus(uuid, { status: CONVERSATION_DEFAULT_STATUSES.SNOOZED, snoozed_until: snoozeDuration })
      fetchConversationStats()
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function upsertTags (v) {
    try {
      await api.upsertTags(conversation.data.uuid, v)
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function updateAssignee (type, v, uuid = conversation.data?.uuid) {
    if (!uuid) return
    try {
      await api.updateAssignee(uuid, type, v)
      fetchConversationStats()
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function removeAssignee (type, uuid = conversation.data?.uuid) {
    if (!uuid) return
    try {
      await api.removeAssignee(uuid, type)
      if (conversation.data?.uuid === uuid) {
        conversation.data[`assigned_${type}_id`] = null
      }
      fetchConversationStats()
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  async function updateAssigneeLastSeen (uuid) {
    markConversationAsRead(uuid)
    try {
      await api.updateAssigneeLastSeen(uuid)
    } catch (error) {
      // pass
    }
  }

  function isConversationInList (uuid) {
    return conversations.data?.find(c => c.uuid === uuid) ? true : false
  }

  // Pending notification UUIDs for new conversations not yet in list (refresh is debounced).
  // Checked after processConversationListResponse adds conversations to the list.
  const pendingNotificationUUIDs = new Set()

  function addPendingNotification (uuid) {
    pendingNotificationUUIDs.add(uuid)
  }

  // Throttled to cap list refreshes during bursts of WS events (leading + trailing).
  const throttledFetchFirstPage = useThrottleFn(fetchFirstPageConversations, 30000, true)
  const throttledFetchConversationStats = useThrottleFn(fetchConversationStats, 30000, true)

  function refreshConversationList () {
    throttledFetchFirstPage()
    refreshConversationStats()
  }

  function refreshConversationStats () {
    throttledFetchConversationStats()
  }

  function updateConversationLastMessage (uuid, message) {
    const conv = conversations.data?.find(c => c.uuid === uuid)
    if (!conv) return
    conv.last_message = message.text_content || message.content || getMediaPreview(message.attachments)
    conv.last_message_at = message.created_at
    conv.last_message_sender = message.sender_type
  }

  /**
   * Update conversation message in the cache by fetching it from the server.
   *
   * @param {object} message - Message object with conversation_uuid field
   */
  async function updateConversationMessage (message) {
    if (conversation.data?.uuid !== message.conversation_uuid) {
      // Not the open conversation. If we have cached messages for it,
      // fetch the new message to keep the cache fresh.
      if (messages.data.getLastFetchedPage(message.conversation_uuid) > 0) {
        const fetchedMessage = await fetchMessage(message.conversation_uuid, message.uuid)
        if (fetchedMessage) {
          // Update last message in conversation list (preview)
          updateConversationLastMessage(message.conversation_uuid, fetchedMessage)
        }
      }
      return
    }

    // Open conversation and message not in cache? Fetch from server.
    if (!messages.data.hasMessage(message.conversation_uuid, message.uuid)) {
      // Match echo_id to pending message and swap its UUID so mergeMessageUpdate can find it.
      const echoId = message.echo_id
      if (echoId && messages.data.hasMessage(message.conversation_uuid, echoId)) {
        messages.data.updateMessage(message.conversation_uuid, echoId, { uuid: message.uuid })
        incrementMessageVersion()
        updateAssigneeLastSeen(message.conversation_uuid)
        return
      }

      const fetchedMessage = await fetchMessage(message.conversation_uuid, message.uuid)
      if (fetchedMessage) {
        updateConversationLastMessage(message.conversation_uuid, fetchedMessage)
        setTimeout(() => {
          emitter.emit(EMITTER_EVENTS.NEW_MESSAGE, {
            conversation_uuid: message.conversation_uuid,
            message: fetchedMessage
          })
        }, 100)
      }

      if (!document.hidden) {
        updateAssigneeLastSeen(message.conversation_uuid)
      }
    }
  }

  function addPendingMessage (conversationUUID, content, isPrivate, author, attachments = [], textContent = '', meta = {}) {
    const pendingMessage = {
      uuid: `pending-${Date.now()}`,
      type: 'outgoing',
      status: 'pending',
      content,
      text_content: textContent,
      content_type: 'html',
      private: isPrivate,
      sender_type: 'agent',
      sender_id: author.id,
      conversation_uuid: conversationUUID,
      created_at: new Date().toISOString(),
      author,
      attachments: attachments.map(a => ({
        uuid: a.uuid,
        name: a.filename || a.name,
        size: a.size,
        content_type: a.content_type,
        url: a.url,
        disposition: a.disposition
      })),
      meta
    }
    messages.data.addMessage(conversationUUID, pendingMessage)
    incrementMessageVersion()
    setTimeout(() => {
      emitter.emit(EMITTER_EVENTS.NEW_MESSAGE, {
        conversation_uuid: conversationUUID,
        message: pendingMessage
      })
    }, 0)

    // Safety net: auto-remove after 10 seconds if still pending.
    const tempId = pendingMessage.uuid
    setTimeout(() => {
      if (messages.data.hasMessage(conversationUUID, tempId)) {
        messages.data.removeMessage(conversationUUID, tempId)
        incrementMessageVersion()
      }
    }, 10000)

    return pendingMessage.uuid
  }

  function replacePendingMessage (conversationUUID, tempUUID, realMessage) {
    if (messages.data.hasMessage(conversationUUID, realMessage.uuid)) {
      messages.data.removeMessage(conversationUUID, tempUUID)
    } else {
      messages.data.updateMessage(conversationUUID, tempUUID, realMessage)
    }
    incrementMessageVersion()
  }

  function removePendingMessage (conversationUUID, tempUUID) {
    messages.data.removeMessage(conversationUUID, tempUUID)
    incrementMessageVersion()
  }

  function addNewConversation (conversation) {
    if (!isConversationInList(conversation.uuid)) {
      // Fetch list of conversations again.
      fetchFirstPageConversations()
    }
  }

  function mergeMessageUpdate (data) {
    const { conversation_uuid, uuid, ...fields } = data
    if (!messages.data.hasMessage(conversation_uuid, uuid)) return
    messages.data.updateMessage(conversation_uuid, uuid, fields)
    incrementMessageVersion()
  }

  function mergeConversationUpdate (update) {
    if (conversation.data?.uuid === update.uuid) {
      deepMerge(conversation.data, update)
    }
    const existing = conversations?.data?.find(c => c.uuid === update.uuid)
    if (existing) {
      deepMerge(existing, update)
    }
    refreshConversationStats()
  }

  function mergeContactUpdate (update) {
    const { contact_id, ...fields } = update
    if (conversation.data?.contact_id === contact_id) {
      if (!conversation.data.contact) conversation.data.contact = {}
      deepMerge(conversation.data.contact, fields)
    }
    conversations?.data?.forEach(c => {
      if (c.contact_id === contact_id) {
        if (!c.contact) c.contact = {}
        deepMerge(c.contact, fields)
      }
    })
  }

  function emitAgentValidationCompleted (payload) {
    emitter.emit(EMITTER_EVENTS.AGENT_VALIDATION_COMPLETED, payload)
  }

  function emitTagClassificationCompleted (payload) {
    emitter.emit(EMITTER_EVENTS.TAG_CLASSIFICATION_COMPLETED, payload)
  }

  function emitCustomerIdentificationCompleted (payload) {
    emitter.emit(EMITTER_EVENTS.CUSTOMER_IDENTIFICATION_COMPLETED, payload)
  }

  // Clears the list and pagination state so the next fetch starts fresh (removes stale rows).
  function resetConversations () {
    conversations.data = []
    conversations.page = 1
    seenConversationUUIDs = new Map()
    contextSeq++
  }

  /** Macros set for new conversation or an open conversation **/
  function setMacro (macro, context) {
    macros.value[context] = macro
  }

  function setMacroActions (actions, context) {
    if (!macros.value[context]) {
      macros.value[context] = {}
    }
    macros.value[context].actions = actions
  }

  function getMacro (context) {
    return macros.value[context] || {}
  }

  function removeMacroAction (action, context) {
    if (!macros.value[context]) return
    macros.value[context].actions = macros.value[context].actions.filter(a => a.type !== action.type)
  }

  function resetMacro (context) {
    macros.value = { ...macros.value, [context]: {} }
  }

  // Typing indicators
  function updateTypingStatus (typingData) {
    const { conversation_uuid, is_typing } = typingData

    // Only update typing status for the current conversation
    if (conversation.data?.uuid !== conversation_uuid) return

    if (typingTimeout) {
      clearTimeout(typingTimeout)
      typingTimeout = null
    }

    conversation.isTyping = is_typing

    if (is_typing) {
      typingTimeout = setTimeout(() => {
        conversation.isTyping = false
        typingTimeout = null
      }, TYPING_RECEIVE_TIMEOUT)
    }
  }

  function sendTyping (isTyping, otherAttributes = {}) {
    // Send typing websocket message only if a conversation is open
    if (conversation.data?.uuid) {
      sendTypingIndicator(conversation.data.uuid, isTyping, otherAttributes.isPrivateMessage)
    }
  }

  // Fetch all drafts for the current user
  async function fetchAllDrafts () {
    try {
      const resp = await api.getAllDrafts()
      const newDrafts = new Map()
      if (resp.data?.data) {
        for (const draft of resp.data.data) {
          newDrafts.set(draft.conversation_uuid, draft)
        }
      }
      drafts.value = newDrafts
    } catch (e) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(e).message
      })
    }
  }

  // Get draft for a specific conversation
  function getDraft (uuid) {
    return drafts.value.get(uuid)
  }

  // Set draft for a specific conversation
  function setDraft (uuid, draft) {
    drafts.value.set(uuid, draft)
    // Trigger reactivity
    drafts.value = new Map(drafts.value)
  }

  // Remove draft for a specific conversation
  function removeDraft (uuid) {
    drafts.value.delete(uuid)
    // Trigger reactivity
    drafts.value = new Map(drafts.value)
  }

  // Check if a conversation has a draft
  function hasDraft (uuid) {
    return drafts.value.has(uuid)
  }


  function getMediaPreview (attachments) {
    if (!attachments?.length) return ''
    const contentType = attachments[0].content_type || ''
    const i18n = getI18n()
    const t = i18n?.global?.t || ((key) => key.split('.').pop())

    if (contentType.startsWith('image/')) return t('globals.terms.image')
    if (contentType.startsWith('video/')) return t('globals.terms.video')
    if (contentType.startsWith('audio/')) return t('globals.terms.audio')
    return t('globals.terms.file')
  }

  return {
    macros,
    stats,
    conversations,
    conversation,
    messages,
    conversationsList,
    conversationMessages,
    currentConversationHasMoreMessages,
    isConversationOpen,
    current,
    currentContactName,
    currentTo,
    currentBCC,
    currentCC,
    recomputeRecipientsForInbox,
    isConversationInList,
    addPendingNotification,
    mergeConversationUpdate,
    emitAgentValidationCompleted,
    emitTagClassificationCompleted,
    emitCustomerIdentificationCompleted,
    mergeContactUpdate,
    addNewConversation,
    getContactFullName,
    fetchNextMessages,
    fetchNextConversations,
    mergeMessageUpdate,
    updateAssigneeLastSeen,
    markAsUnread,
    updateConversationMessage,
    snoozeConversation,
    fetchConversation,
    fetchConversationsList,
    fetchMessages,
    upsertTags,
    updateAssignee,
    updatePriority,
    updateStatus,
    refreshConversationList,
    resetConversations,
    updateConversationLastMessage,
    fetchFirstPageConversations,
    fetchStatuses,
    fetchPriorities,
    fetchConversationStats,
    refreshConversationStats,
    setListSortField,
    setListStatus,
    removeMacroAction,
    getMacro,
    setMacro,
    resetMacro,
    setMacroActions,
    removeAssignee,
    getListSortField,
    getListStatus,
    statuses,
    priorities,
    priorityOptions,
    statusOptionsNoSnooze,
    statusOptions,
    updateTypingStatus,
    sendTyping,
    drafts,
    fetchAllDrafts,
    getDraft,
    setDraft,
    removeDraft,
    hasDraft,
    addPendingMessage,
    replacePendingMessage,
    removePendingMessage,
    viewers,
    setViewers
  }
})
