function stringValue (value) {
  if (value == null) return ''
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (typeof value === 'object') {
    if (value.Valid === false || value.valid === false) return ''
    return stringValue(value.String ?? value.string ?? value.Value ?? value.value ?? '')
  }
  return ''
}

/**
 * Returns the display name for a conversation. Priority chain:
 *   1. Customer display name when linked
 *   2. Contact full name
 *   3. Contact email
 *   4. Empty-state placeholder
 */
export function getConversationDisplayName (conversation) {
  if (!conversation) return ''

  if (hasLinkedCustomer(conversation)) {
    const customerName = getCustomerDisplayName(conversation.customer)
    if (customerName) return customerName
  }

  const contact = conversation.contact
  if (contact) {
    const fullName = [
      stringValue(contact.first_name),
      stringValue(contact.last_name)
    ].filter(Boolean).join(' ').trim()
    if (fullName) return fullName

    const email = stringValue(contact.email)
    if (email) return email
  }

  return ''
}

/**
 * Returns true if the conversation has a linked customer.
 */
export function hasLinkedCustomer (conversation) {
  return Boolean(stringValue(conversation?.customer_id))
}

/**
 * Returns the display name for a customer record. Priority chain:
 *   1. company
 *   2. firstname + lastname
 *   3. customer_id
 */
export function getCustomerDisplayName (customer) {
  if (!customer) return ''

  const company = stringValue(customer.company)
  if (company) return company

  const personName = [
    stringValue(customer.firstname),
    stringValue(customer.lastname)
  ].filter(Boolean).join(' ').trim()
  if (personName) return personName

  return stringValue(customer.customer_id)
}
