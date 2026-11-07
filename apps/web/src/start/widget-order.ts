export const widgetLimit = 7

export function readWidgetOrder<T extends string>(value: unknown, registered: readonly T[]): T[] {
  if (!Array.isArray(value)) return []
  const ids = new Map<string, T>(registered.map(id => [id, id]))
  const order = new Set<T>()
  for (const item of value) {
    const id = typeof item === 'string' ? ids.get(item) : undefined
    if (id !== undefined) order.add(id)
    if (order.size === widgetLimit) break
  }
  return [...order]
}

export function moveWidget<T>(order: T[], id: T, offset: -1 | 1): T[] {
  const index = order.indexOf(id)
  const target = index + offset
  if (index < 0 || target < 0 || target >= order.length) return order
  const next = [...order]
  next[index] = order[target]
  next[target] = id
  return next
}

export function toggleWidgetInOrder<T extends string>(order: T[], id: T, registered: readonly T[]): T[] {
  if (order.includes(id)) return order.filter(item => item !== id)
  return order.length < widgetLimit && registered.includes(id) ? [...order, id] : order
}

export function sameWidgetSet<T>(current: T[], proposed: T[]) {
  return proposed.length === current.length && new Set(proposed).size === current.length && proposed.every(id => current.includes(id))
}
