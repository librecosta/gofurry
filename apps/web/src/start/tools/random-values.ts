export const maxRandomInteger = 1_000_000_000
export const maxDice = 20
export const diceFaces = [4, 6, 8, 10, 12, 20, 100]
export const maxCandidateLength = 4000
export const maxCandidates = 100

export function parseInteger(input: string, minimum: number, maximum: number) {
  if (!/^[+-]?\d+$/.test(input.trim())) return null
  const value = Number(input)
  return Number.isSafeInteger(value) && value >= minimum && value <= maximum ? value : null
}

export function randomInteger(minimum: number, maximum: number): number {
  if (!Number.isSafeInteger(minimum) || !Number.isSafeInteger(maximum)
    || minimum < -maxRandomInteger || maximum > maxRandomInteger || minimum > maximum) throw new RangeError('Invalid random range')
  const size = maximum - minimum + 1
  const space = 2 ** 32
  const cutoff = space - space % size
  const sample = new Uint32Array(1)
  // Rejection keeps each outcome equally likely even when size does not divide 2³².
  do { crypto.getRandomValues(sample) } while (sample[0] >= cutoff)
  return minimum + sample[0] % size
}

export function parseCandidates(text: string) {
  if (text.length > maxCandidateLength) return null
  const choices = text.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
  return choices.length && choices.length <= maxCandidates ? choices : null
}
