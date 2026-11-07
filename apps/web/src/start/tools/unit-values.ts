export const units = {
  length: [
    { symbol: 'mm', factor: 0.001 }, { symbol: 'cm', factor: 0.01 }, { symbol: 'm', factor: 1 }, { symbol: 'km', factor: 1000 },
    { symbol: 'in', factor: 0.0254 }, { symbol: 'ft', factor: 0.3048 }, { symbol: 'yd', factor: 0.9144 }, { symbol: 'mi', factor: 1609.344 },
  ],
  area: [
    { symbol: 'cm²', factor: 0.0001 }, { symbol: 'm²', factor: 1 }, { symbol: 'km²', factor: 1_000_000 }, { symbol: 'ha', factor: 10_000 },
    { symbol: 'in²', factor: 0.0254 ** 2 }, { symbol: 'ft²', factor: 0.3048 ** 2 }, { symbol: 'acre', factor: 4046.8564224 },
  ],
  mass: [
    { symbol: 'mg', factor: 0.000001 }, { symbol: 'g', factor: 0.001 }, { symbol: 'kg', factor: 1 }, { symbol: 't', factor: 1000 },
    { symbol: 'oz', factor: 0.45359237 / 16 }, { symbol: 'lb', factor: 0.45359237 },
  ],
  temperature: [
    { symbol: '°C', toBase: (value: number) => value, fromBase: (value: number) => value },
    { symbol: '°F', toBase: (value: number) => (value - 32) / 1.8, fromBase: (value: number) => value * 1.8 + 32 },
    { symbol: 'K', toBase: (value: number) => value - 273.15, fromBase: (value: number) => value + 273.15 },
  ],
  volume: [
    { symbol: 'mL', factor: 0.001 }, { symbol: 'L', factor: 1 }, { symbol: 'm³', factor: 1000 },
    { symbol: 'US fl oz', factor: 3.785411784 / 128 }, { symbol: 'US gal', factor: 3.785411784 },
  ],
} as const

export type UnitCategory = keyof typeof units

export function convertUnit(category: UnitCategory, value: number, source: string, target: string) {
  if (!Number.isFinite(value)) return null
  const from = units[category].find(unit => unit.symbol === source)
  const to = units[category].find(unit => unit.symbol === target)
  if (!from || !to) return null
  if (from === to) return value
  const result = 'factor' in from && 'factor' in to ? value * (from.factor / to.factor)
    : 'toBase' in from && 'fromBase' in to ? to.fromBase(from.toBase(value)) : NaN
  return Number.isFinite(result) ? result : null
}

export function parseQuantity(input: string) {
  const normalized = input.trim().replace('−', '-')
  if (!/^[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?$/i.test(normalized)) return null
  const value = Number(normalized)
  return Number.isFinite(value) ? value : null
}
