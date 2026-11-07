export const expressionLimit = 160
type ErrorCode = 'invalid' | 'zero' | 'overflow'
type Calculation = { value: number } | { error: ErrorCode }

export function calculate(expression: string): Calculation {
  if (expression.length > expressionLimit) return { error: 'invalid' }
  const source = expression.replaceAll('×', '*').replaceAll('÷', '/').replaceAll('−', '-')
  const numeric = /(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?/iy
  let index = 0

  function peek() {
    while (/\s/.test(source[index] ?? '')) index++
    return source[index]
  }

  function finite(value: number) {
    if (!Number.isFinite(value)) throw 'overflow'
    return value
  }

  function operand(): number {
    const token = peek()
    if (token === '+' || token === '-') {
      index++
      const value = operand()
      return token === '-' ? -value : value
    }
    if (token === '(') {
      index++
      const value = sum()
      if (peek() !== ')') throw 'invalid'
      index++
      return value
    }
    numeric.lastIndex = index
    const match = numeric.exec(source)
    if (!match) throw 'invalid'
    index = numeric.lastIndex
    return finite(Number(match[0]))
  }

  function product(): number {
    let value = operand()
    for (let token = peek(); token === '*' || token === '/'; token = peek()) {
      index++
      const next = operand()
      if (token === '/' && next === 0) throw 'zero'
      value = finite(token === '*' ? value * next : value / next)
    }
    return value
  }

  function sum(): number {
    let value = product()
    for (let token = peek(); token === '+' || token === '-'; token = peek()) {
      index++
      const next = product()
      value = finite(token === '+' ? value + next : value - next)
    }
    return value
  }

  try {
    const value = sum()
    return peek() === undefined ? { value } : { error: 'invalid' }
  } catch (error) {
    return { error: error === 'zero' || error === 'overflow' ? error : 'invalid' }
  }
}

export function displayNumber(value: number) {
  const rounded = Number(value.toPrecision(12))
  return String(Number.isFinite(rounded) ? rounded : value)
}
