import { useState } from 'react'
import { ArrowsLeftRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowsLeftRight'
import { usePreferences } from '../../preferences'
import { cx } from '../../styles/classes'
import controls from '../controls.module.scss'
import { displayNumber } from './calculate'
import { CopyResult } from './copy-result'
import { convertUnit, parseQuantity, units, type UnitCategory } from './unit-values'
import styles from './tools.module.scss'

const categories = ['length', 'area', 'mass', 'temperature', 'volume'] as const
const defaults: Record<UnitCategory, [string, string]> = {
  length: ['m', 'cm'], area: ['m²', 'ft²'], mass: ['kg', 'lb'], temperature: ['°C', '°F'], volume: ['L', 'mL'],
}
const labels = {
  zh: {
    category: '分类', length: '长度', area: '面积', mass: '质量', temperature: '温度', volume: '体积',
    value: '数值', from: '来源单位', to: '目标单位', swap: '交换单位', result: '换算结果', invalid: '请输入有效数值。', overflow: '结果超出数值范围。',
  },
  en: {
    category: 'Category', length: 'Length', area: 'Area', mass: 'Mass', temperature: 'Temperature', volume: 'Volume',
    value: 'Value', from: 'From unit', to: 'To unit', swap: 'Swap units', result: 'Conversion result', invalid: 'Enter a valid number.', overflow: 'The result is out of range.',
  },
}

export default function UnitConverter() {
  const { locale } = usePreferences()
  const m = labels[locale]
  const [category, setCategory] = useState<UnitCategory>('length')
  const [pair, setPair] = useState(defaults.length)
  const [quantity, setQuantity] = useState('1')
  const [source, target] = pair
  const number = parseQuantity(quantity)
  const converted = number === null ? null : convertUnit(category, number, source, target)
  const value = converted === null ? '' : displayNumber(converted)
  const options = units[category].map(({ symbol }) => <option key={symbol} value={symbol}>{symbol}</option>)
  const result = value ? `${value} ${target}` : quantity.trim() ? (number === null ? m.invalid : m.overflow) : ''

  return <div className={styles.form}>
    <div className={styles.pair}>
      <label className={styles.field}>{m.category}<select className={styles.input} value={category} onChange={event => {
        const next = event.target.value as UnitCategory
        setCategory(next)
        setPair(defaults[next])
      }}>
        {categories.map(id => <option key={id} value={id}>{m[id]}</option>)}
      </select></label>
      <label className={styles.field}>{m.value}
        <input className={styles.input} inputMode="decimal" maxLength={40} value={quantity} onChange={event => setQuantity(event.target.value)} />
      </label>
    </div>
    <div className={styles.unitPair}>
      <label className={styles.field}>{m.from}
        <select className={styles.input} value={source} onChange={event => setPair([event.target.value, target])}>{options}</select>
      </label>
      <button type="button" aria-label={m.swap} className={cx(controls.glass, styles.swap)} onClick={() => setPair([target, source])}>
        <ArrowsLeftRightIcon size={17} aria-hidden="true" />
      </button>
      <label className={styles.field}>{m.to}
        <select className={styles.input} value={target} onChange={event => setPair([source, event.target.value])}>{options}</select>
      </label>
    </div>
    <div className={styles.result}><output aria-label={m.result}>{result}</output><CopyResult key={result} value={value ? result : ''} /></div>
  </div>
}
