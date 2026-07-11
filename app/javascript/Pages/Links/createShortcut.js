export function isCreateShortcut(event) {
  return event.key === 'Enter' && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey
}
