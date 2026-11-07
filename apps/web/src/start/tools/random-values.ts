export const maxRandomInteger = 1_000_000_000
export const maxDice = 20
export const maxDiceFaces = 1000
export const maxPasswordLength = 50
export const maxDecimalPlaces = 6
export const maxCandidateLength = 4000
export const maxCandidates = 100
export const passwordGroups = {
  letters: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz',
  digits: '0123456789',
  symbols: '!@#$%&*+-=?_',
} as const

export function parseInteger(input: string, minimum: number, maximum: number) {
  if (!/^[+-]?\d+$/.test(input.trim())) return null
  const value = Number(input)
  return Number.isSafeInteger(value) && value >= minimum && value <= maximum ? value : null
}

export function randomInteger(minimum: number, maximum: number): number {
  if (!Number.isSafeInteger(minimum) || !Number.isSafeInteger(maximum)
    || minimum > maximum || !Number.isSafeInteger(maximum - minimum + 1)) throw new RangeError('Invalid random range')
  const size = maximum - minimum + 1
  if (size === 1) return minimum
  const wide = size > 2 ** 32
  const space = wide ? 2 ** 53 : 2 ** 32
  const cutoff = space - space % size
  const sample = new Uint32Array(wide ? 2 : 1)
  let value: number
  // Reject the incomplete bucket to avoid modulo bias, including for wide fixed-point ranges.
  do {
    crypto.getRandomValues(sample)
    value = wide ? (sample[1] & 0x1fffff) * 2 ** 32 + sample[0] : sample[0]
  } while (value >= cutoff)
  return minimum + value % size
}

export function randomPassword(length: number, alphabet: string) {
  return Array.from({ length }, () => alphabet[randomInteger(0, alphabet.length - 1)]).join('')
}

function decimalBound(input: string, places: number, lower: boolean) {
  const match = /^([+-]?)(\d*)(?:\.(\d*))?$/.exec(input.trim())
  if (!match) return null
  const [, sign, whole, fraction = ''] = match
  if (!whole && !fraction) return null
  const integer = Number(whole)
  if (integer > maxRandomInteger || (integer === maxRandomInteger && /[1-9]/.test(fraction))) return null
  const scaled = integer * 10 ** places + Number(fraction.slice(0, places).padEnd(places, '0'))
  const remainder = /[1-9]/.test(fraction.slice(places))
  // Round inward so every generated tick is within the exact decimal bounds.
  return sign === '-' ? -scaled - (remainder && !lower ? 1 : 0) : scaled + (remainder && lower ? 1 : 0)
}

export function randomDecimalRange(minimum: string, maximum: string, places: number): { value: string } | { error: 'places' | 'bounds' | 'range' } {
  if (!Number.isInteger(places) || places < 0 || places > maxDecimalPlaces) return { error: 'places' }
  const low = decimalBound(minimum, places, true)
  const high = decimalBound(maximum, places, false)
  if (low === null || high === null) return { error: 'bounds' }
  if (low > high) return { error: 'range' }
  return { value: (randomInteger(low, high) / 10 ** places).toFixed(places) }
}

export function parseCandidates(text: string) {
  if (text.length > maxCandidateLength) return null
  const choices = text.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
  return choices.length && choices.length <= maxCandidates ? choices : null
}
