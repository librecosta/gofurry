import { useState } from 'react'
import { Tabs } from '@base-ui/react/tabs'
import { usePreferences } from '../../preferences'
import { cx } from '../../styles/classes'
import controls from '../controls.module.scss'
import { diceFaces, maxCandidateLength, maxDice, maxRandomInteger, parseCandidates, parseInteger, randomInteger } from './random-values'
import styles from './tools.module.scss'

const modes = ['integer', 'dice', 'pick'] as const
type Mode = typeof modes[number]
const labels = {
  zh: {
    modes: '随机工具类型', integer: '随机整数', dice: '掷骰子', pick: '随机抽取', minimum: '最小值', maximum: '最大值',
    count: '骰子数量', faces: '骰子面数', candidates: '候选项（每行一项）', run: '生成结果', total: '合计',
    invalidRange: '请输入 −10 亿至 10 亿之间的整数，最小值不能大于最大值。',
    invalidDice: '请输入 1 至 20 颗骰子，并选择面数。', invalidChoices: '请输入 1 至 100 个候选项，总长度不能超过 4000 字符。',
  },
  en: {
    modes: 'Random tool modes', integer: 'Integer', dice: 'Dice', pick: 'Pick an item', minimum: 'Minimum', maximum: 'Maximum',
    count: 'Dice count', faces: 'Sides per die', candidates: 'Candidates (one per line)', run: 'Generate', total: 'Total',
    invalidRange: 'Use integers from −1 billion to 1 billion. The minimum must not exceed the maximum.',
    invalidDice: 'Enter 1 to 20 dice and select a number of sides.', invalidChoices: 'Enter 1 to 100 candidates with at most 4,000 characters in total.',
  },
}

export default function RandomTools() {
  const { locale } = usePreferences()
  const m = labels[locale]
  const [mode, setMode] = useState<Mode>('integer')
  const [minimum, setMinimum] = useState('1')
  const [maximum, setMaximum] = useState('100')
  const [count, setCount] = useState('2')
  const [faces, setFaces] = useState(6)
  const [candidates, setCandidates] = useState('')
  const [result, setResult] = useState('')

  function generate() {
    if (mode === 'integer') {
      const min = parseInteger(minimum, -maxRandomInteger, maxRandomInteger)
      const max = parseInteger(maximum, -maxRandomInteger, maxRandomInteger)
      setResult(min === null || max === null || min > max ? m.invalidRange : String(randomInteger(min, max)))
    } else if (mode === 'dice') {
      const amount = parseInteger(count, 1, maxDice)
      if (amount === null || !diceFaces.includes(faces)) { setResult(m.invalidDice); return }
      const rolls = Array.from({ length: amount }, () => randomInteger(1, faces))
      setResult(`${rolls.join(' + ')} · ${m.total} ${rolls.reduce((sum, roll) => sum + roll, 0)}`)
    } else {
      const choices = parseCandidates(candidates)
      setResult(choices ? choices[randomInteger(0, choices.length - 1)] : m.invalidChoices)
    }
  }

  const numberField = (label: string, value: string, update: (value: string) => void) => <label className={styles.field}>{label}
    <input className={styles.input} inputMode="numeric" value={value} maxLength={24}
      onChange={event => { update(event.target.value); setResult('') }} />
  </label>

  return <form className={styles.form} onSubmit={event => { event.preventDefault(); generate() }}>
    <Tabs.Root value={mode} onValueChange={value => { setMode(value as Mode); setResult('') }} className={styles.form}>
      <Tabs.List aria-label={m.modes} className={styles.tabs}>
        {modes.map(id => <Tabs.Tab key={id} value={id} className={cx(controls.glass, styles.button, styles.tab)}>{m[id]}</Tabs.Tab>)}
      </Tabs.List>
      <Tabs.Panel value="integer" className={styles.pair}>
        {numberField(m.minimum, minimum, setMinimum)}
        {numberField(m.maximum, maximum, setMaximum)}
      </Tabs.Panel>
      <Tabs.Panel value="dice" className={styles.pair}>
        {numberField(m.count, count, setCount)}
        <label className={styles.field}>{m.faces}<select className={styles.input} value={faces}
          onChange={event => { setFaces(Number(event.target.value)); setResult('') }}>
          {diceFaces.map(face => <option key={face} value={face}>{face}</option>)}
        </select></label>
      </Tabs.Panel>
      <Tabs.Panel value="pick"><label className={styles.field}>{m.candidates}
        <textarea className={styles.input} rows={4} maxLength={maxCandidateLength + 1} value={candidates}
          onChange={event => { setCandidates(event.target.value); setResult('') }} />
      </label></Tabs.Panel>
    </Tabs.Root>
    <button type="submit" className={cx(controls.glass, styles.button)}>{m.run}</button>
    <output className={styles.randomResult}>{result}</output>
  </form>
}
