import { useState } from 'react'
import { BackspaceIcon } from '@phosphor-icons/react/dist/ssr/Backspace'
import { usePreferences } from '../../preferences'
import { cx } from '../../styles/classes'
import controls from '../controls.module.scss'
import { calculate, displayNumber, expressionLimit } from './calculate'
import { CopyResult } from './copy-result'
import styles from './tools.module.scss'

const keys = ['clear', '(', ')', 'delete', '7', '8', '9', '÷', '4', '5', '6', '×', '1', '2', '3', '−', '0', '.', '=', '+']
const labels = {
  zh: { expression: '表达式', result: '结果', clear: '清除', delete: '退格', equals: '计算', invalid: '表达式无效。', zero: '不能除以零。', overflow: '结果超出数值范围。' },
  en: { expression: 'Expression', result: 'Result', clear: 'Clear', delete: 'Backspace', equals: 'Calculate', invalid: 'Enter a valid expression.', zero: 'Cannot divide by zero.', overflow: 'The result is out of range.' },
}

export default function Calculator() {
  const { locale } = usePreferences()
  const m = labels[locale]
  const [expression, setExpression] = useState('')
  const [result, setResult] = useState<ReturnType<typeof calculate> | null>(null)
  const value = result && 'value' in result ? displayNumber(result.value) : ''

  function press(key: string) {
    if (key === 'clear') setExpression('')
    else if (key === 'delete') setExpression(expression.slice(0, -1))
    else {
      const base = value ? ('+−×÷'.includes(key) ? value : '') : expression
      setExpression((base + key).slice(0, expressionLimit))
    }
    setResult(null)
  }

  return <form className={styles.form} onSubmit={event => { event.preventDefault(); setResult(calculate(expression)) }}>
    <label className={styles.field}>{m.expression}
      <input className={styles.input} value={expression} maxLength={expressionLimit} autoComplete="off" spellCheck={false}
        onChange={event => { setExpression(event.target.value); setResult(null) }} />
    </label>
    <div className={cx(styles.result, styles.calculatorResult)}>
      <output aria-label={m.result}>{result && 'error' in result ? m[result.error] : value || <span aria-hidden="true">—</span>}</output>
      <CopyResult key={value} value={value} />
    </div>
    <div className={styles.keypad}>
      {keys.map(key => <button key={key} type={key === '=' ? 'submit' : 'button'} className={cx(controls.glass, styles.button)}
        aria-label={key === '=' ? m.equals : key === 'clear' || key === 'delete' ? m[key] : undefined}
        onClick={key === '=' ? undefined : () => press(key)}>
        {key === 'clear' ? 'AC' : key === 'delete' ? <BackspaceIcon size={18} aria-hidden="true" /> : key}
      </button>)}
    </div>
  </form>
}
