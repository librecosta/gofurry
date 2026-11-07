import { useState } from 'react'
import { Checkbox } from '@base-ui/react/checkbox'
import { Tabs } from '@base-ui/react/tabs'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr/ArrowsClockwise'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { motion } from 'motion/react'
import { useMotionTiming } from '../../motion'
import { usePreferences } from '../../preferences'
import { cx } from '../../styles/classes'
import controls from '../controls.module.scss'
import { CopyResult } from './copy-result'
import {
  maxCandidateLength, maxDecimalPlaces, maxDice, maxDiceFaces, maxPasswordLength,
  parseCandidates, parseInteger, passwordGroups, randomDecimalRange, randomInteger, randomPassword,
} from './random-values'
import styles from './tools.module.scss'

const modes = ['password', 'number', 'dice', 'pick'] as const
type Mode = typeof modes[number]
const charOptions = { letters: 'ABC / abc', digits: '123', symbols: '#$&' }
const charTypes = ['letters', 'digits', 'symbols'] as const
const labels = {
  zh: {
    modes: '随机工具类型', password: '随机密码', number: '随机数', dice: '掷骰子', pick: '随机抽取',
    length: '密码长度', characters: '字符类型', letters: '字母', digits: '数字', symbols: '特殊字符',
    minimum: '最小值', maximum: '最大值', places: '小数位数',
    count: '骰子数量', faces: '骰子面数', candidates: '候选项（每行一项）',
    run: '生成', rerun: '重新生成', total: '合计', result: '结果',
    invalidLength: '密码长度必须是 1 至 50 的整数。', invalidGroups: '请至少选择一类字符。',
    invalidBounds: '请输入 −10 亿至 10 亿之间的数字。', invalidPlaces: '小数位数必须是 0 至 6 的整数。',
    invalidRange: '此范围内没有符合所选精度的数字，请检查上下限和小数位数。',
    invalidDice: '请输入 1 至 20 颗骰子、每颗 2 至 1000 面。',
    invalidChoices: '请输入 1 至 100 个候选项，总长度不能超过 4000 字符。',
  },
  en: {
    modes: 'Random tool modes', password: 'Password', number: 'Number', dice: 'Dice', pick: 'Pick',
    length: 'Password length', characters: 'Character types', letters: 'Letters', digits: 'Digits', symbols: 'Special characters',
    minimum: 'Minimum', maximum: 'Maximum', places: 'Decimal places',
    count: 'Dice count', faces: 'Sides per die', candidates: 'Candidates (one per line)',
    run: 'Generate', rerun: 'Regenerate', total: 'Total', result: 'Result',
    invalidLength: 'Enter a password length from 1 to 50.', invalidGroups: 'Select at least one character type.',
    invalidBounds: 'Use numbers from −1 billion to 1 billion.', invalidPlaces: 'Enter 0 to 6 decimal places.',
    invalidRange: 'No number fits this range and precision. Check the bounds and decimal places.',
    invalidDice: 'Enter 1 to 20 dice with 2 to 1,000 sides each.',
    invalidChoices: 'Enter 1 to 100 candidates with at most 4,000 characters in total.',
  },
}

export default function RandomTools() {
  const { locale } = usePreferences()
  const m = labels[locale]
  const [mode, setMode] = useState<Mode>('password')
  const [length, setLength] = useState('16')
  const [enabled, setEnabled] = useState({ letters: true, digits: true, symbols: false })
  const [minimum, setMinimum] = useState('1')
  const [maximum, setMaximum] = useState('100')
  const [places, setPlaces] = useState('0')
  const [count, setCount] = useState('2')
  const [faces, setFaces] = useState('6')
  const [candidates, setCandidates] = useState('')
  const [result, setResult] = useState<{ value: string } | { error: string } | null>(null)
  const [generation, setGeneration] = useState(0)
  const transition = useMotionTiming(0.3)
  const value = result && 'value' in result ? result.value : ''

  function generate() {
    const succeed = (value: string) => { setResult({ value }); setGeneration(previous => previous + 1) }
    const fail = (error: string) => setResult({ error })
    switch (mode) {
      case 'password': {
        const size = parseInteger(length, 1, maxPasswordLength)
        const alphabet = charTypes.filter(type => enabled[type]).map(type => passwordGroups[type]).join('')
        if (size === null) return fail(m.invalidLength)
        if (!alphabet) return fail(m.invalidGroups)
        return succeed(randomPassword(size, alphabet))
      }
      case 'number': {
        const number = randomDecimalRange(minimum, maximum, parseInteger(places, 0, maxDecimalPlaces) ?? NaN)
        if ('error' in number) return fail({ places: m.invalidPlaces, bounds: m.invalidBounds, range: m.invalidRange }[number.error])
        return succeed(number.value)
      }
      case 'dice': {
        const amount = parseInteger(count, 1, maxDice)
        const sides = parseInteger(faces, 2, maxDiceFaces)
        if (amount === null || sides === null) return fail(m.invalidDice)
        const rolls = Array.from({ length: amount }, () => randomInteger(1, sides))
        return succeed(`${rolls.join(' + ')} · ${m.total} ${rolls.reduce((sum, roll) => sum + roll, 0)}`)
      }
      case 'pick': {
        const choices = parseCandidates(candidates)
        if (!choices) return fail(m.invalidChoices)
        return succeed(choices[randomInteger(0, choices.length - 1)])
      }
    }
  }

  const numberField = (label: string, current: string, update: (value: string) => void, inputMode: 'numeric' | 'decimal' = 'numeric') =>
    <label className={styles.field}>{label}
      <input className={styles.input} inputMode={inputMode} value={current} maxLength={32} autoComplete="off"
        onChange={event => { update(event.target.value); setResult(null) }} />
    </label>

  return <form className={styles.form} onSubmit={event => { event.preventDefault(); generate() }}>
    <Tabs.Root value={mode} onValueChange={(value: Mode) => { setMode(value); setResult(null) }} className={styles.form}>
      <Tabs.List aria-label={m.modes} activateOnFocus className={styles.tabs}>
        <motion.span aria-hidden="true" className={styles.tabSelection} initial={false}
          animate={{ x: `${modes.indexOf(mode) * 100}%` }} transition={transition} />
        {modes.map(id => <Tabs.Tab key={id} value={id} className={styles.tab}>{m[id]}</Tabs.Tab>)}
      </Tabs.List>
      <Tabs.Panel value="password" className={styles.form}>
        {numberField(m.length, length, setLength)}
        <fieldset className={styles.characters}>
          <legend>{m.characters}</legend>
          <div className={styles.characterOptions}>
            {charTypes.map(type => <label key={type} className={styles.characterOption}>
              <Checkbox.Root checked={enabled[type]} aria-label={m[type]} className={cx(controls.glass, styles.checkbox)}
                onCheckedChange={checked => { setEnabled(current => ({ ...current, [type]: checked })); setResult(null) }}>
                <Checkbox.Indicator><CheckIcon size={12} weight="bold" aria-hidden="true" /></Checkbox.Indicator>
              </Checkbox.Root>
              {charOptions[type]}
            </label>)}
          </div>
        </fieldset>
      </Tabs.Panel>
      <Tabs.Panel value="number" className={styles.form}>
        <div className={styles.pair}>
          {numberField(m.minimum, minimum, setMinimum, 'decimal')}
          {numberField(m.maximum, maximum, setMaximum, 'decimal')}
        </div>
        {numberField(m.places, places, setPlaces)}
      </Tabs.Panel>
      <Tabs.Panel value="dice" className={styles.pair}>
        {numberField(m.count, count, setCount)}
        {numberField(m.faces, faces, setFaces)}
      </Tabs.Panel>
      <Tabs.Panel value="pick"><label className={styles.field}>{m.candidates}
        <textarea className={cx(styles.input, styles.candidateInput)} rows={3} maxLength={maxCandidateLength + 1} value={candidates}
          onChange={event => { setCandidates(event.target.value); setResult(null) }} />
      </label></Tabs.Panel>
    </Tabs.Root>
    <div className={styles.result}>
      <output aria-label={m.result} className={cx(styles.randomOutput, mode === 'password' && !!value && styles.passwordOutput)} dir={mode === 'password' && value ? 'ltr' : undefined}>
        {result && 'error' in result ? result.error : value || <span aria-hidden="true">—</span>}
      </output>
      <div className={styles.resultActions}>
        <CopyResult key={generation} value={value} />
        <button type="submit" className={cx(controls.glass, value ? styles.copy : styles.button)} aria-label={value ? m.rerun : m.run}>
          {value ? <ArrowsClockwiseIcon size={17} aria-hidden="true" /> : m.run}
        </button>
      </div>
    </div>
  </form>
}
