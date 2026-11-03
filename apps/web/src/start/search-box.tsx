import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr/CaretDown'
import { CaretUpIcon } from '@phosphor-icons/react/dist/ssr/CaretUp'
import * as stylex from '@stylexjs/stylex'
import { usePreferences } from '../preferences'
import { tokens } from '../styles/tokens.stylex'
import { controls } from './controls.stylex'
import { useStartMessages } from './messages'
import { categories, defaultPlatform, pickerOrder, platforms, searchURL, type Category } from './search-platforms'

export function SearchBox() {
  const { region, locale } = usePreferences()
  const m = useStartMessages()
  const [category, setCategory] = useState<Category>('search')
  const [selected, setSelected] = useState(defaultPlatform('search', region).id)
  const [order, setOrder] = useState(() => pickerOrder('search', selected))
  const [keyword, setKeyword] = useState('')
  const [expanded, setExpanded] = useState(false)
  const [pointerFocus, setPointerFocus] = useState(false)
  const composing = useRef(false)
  const shell = useRef<HTMLDivElement>(null)
  const picker = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const mobile = useRef<MediaQueryList | null>(null)
  const pickerId = useId()
  const available = platforms[category]
  const target = available.find((item) => item.id === selected)!
  const columns = Math.min(4, order.length)
  const selectedSlot = columns - 1

  // Suggestions deliberately have no transport, debounce, loading state or
  // results until this site owns a real source. Thus blur/category/composition
  // changes cannot leave a pending legacy request or publish stale suggestions.
  useEffect(() => {
    const query = window.matchMedia('(max-width: 767px)')
    mobile.current = query
    const breakpoint = () => {
      if (query.matches) return
      const ownsFocus = picker.current?.contains(document.activeElement)
      setExpanded(false)
      if (ownsFocus) focusCategory()
    }
    query.addEventListener('change', breakpoint)
    return () => { query.removeEventListener('change', breakpoint); mobile.current = null }
  }, [])
  useEffect(() => {
    if (!expanded) return
    const outside = (event: PointerEvent) => { if (!picker.current?.contains(event.target as Node)) setExpanded(false) }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [expanded])

  function focusCategory() {
    shell.current?.querySelector<HTMLButtonElement>('[data-categories] [aria-pressed="true"]')?.focus({ preventScroll: true })
  }
  function chooseCategory(next: Category, button: HTMLButtonElement) {
    const first = defaultPlatform(next, region)
    setCategory(next); setSelected(first.id); setOrder(pickerOrder(next, first.id)); setExpanded(false)
    const track = button.closest<HTMLElement>('[data-category-track]')!
    const item = button.getBoundingClientRect(), bounds = track.getBoundingClientRect()
    if (item.left < bounds.left) track.scrollLeft -= bounds.left - item.left
    else if (item.right > bounds.right) track.scrollLeft += item.right - bounds.right
  }
  function choosePlatform(id: string, fromMobile = false) {
    setSelected(id)
    setOrder((current) => {
      const next = [...current], index = next.indexOf(id)
      ;[next[index], next[selectedSlot]] = [next[selectedSlot], next[index]]
      return next
    })
    if (fromMobile) trigger.current?.focus({ preventScroll: true })
  }
  function groupKeys(event: KeyboardEvent<HTMLElement>, grid = false) {
    if (event.nativeEvent.isComposing) return
    const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
    // Grid order follows visual positions, while the selected trigger keeps its
    // stable DOM identity and anchor when another platform swaps into its slot.
    if (grid) buttons.sort((a, b) => Number(a.dataset.slot) - Number(b.dataset.slot))
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
    const step = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowDown' ? (grid ? columns : 1) : event.key === 'ArrowUp' ? (grid ? -columns : -1) : 0
    if (step || event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + step + buttons.length) % buttons.length
      buttons[next]?.focus({ preventScroll: true })
    }
  }
  function inputKeys(event: KeyboardEvent<HTMLInputElement>) {
    if (composing.current || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) {
      // Safari can clear isComposing before the IME's confirming Enter.
      if (event.key === 'Enter') event.preventDefault()
      return
    }
    if (event.key === 'Escape') { event.preventDefault(); setExpanded(false) }
    // With remote suggestions disabled, Up/Down cannot select hidden/stale data.
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') event.preventDefault()
  }
  const defaultAction = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
  return <div ref={shell} data-start-search {...stylex.props(styles.shell)}>
    <div data-category-track {...stylex.props(styles.categoryTrack)}>
      <div role="group" aria-label={m.categories} data-categories onKeyDown={(event) => groupKeys(event)} {...stylex.props(styles.categories)}>
        {categories.map((id) => <button key={id} type="button" aria-pressed={category === id}
          onClick={(event) => chooseCategory(id, event.currentTarget)} {...stylex.props(controls.glass, styles.category, category === id && controls.selected)}>{m.category[id]}</button>)}
      </div>
    </div>
    <form role="search" aria-label={m.search} method="get" action={defaultAction} target="_blank" rel="noopener noreferrer" acceptCharset="UTF-8"
      onSubmit={(event) => {
        event.preventDefault()
        if (composing.current || !keyword.trim()) return
        window.open(searchURL(target, keyword, locale), '_blank', 'noopener,noreferrer')
        setExpanded(false)
      }} {...stylex.props(styles.search, !pointerFocus && styles.keyboardFocus)}>
      <input type="search" name="q" aria-label={m.search} placeholder={m.search} required pattern={String.raw`.*\S.*`}
        value={keyword} autoComplete="off" autoCapitalize="off" spellCheck={false} enterKeyHint="search"
        onChange={(event) => { setKeyword(event.target.value); setExpanded(false) }} onKeyDown={inputKeys}
        onCompositionStart={() => { composing.current = true; setExpanded(false) }}
        onCompositionEnd={(event) => { composing.current = false; setKeyword(event.currentTarget.value) }}
        onFocus={() => setExpanded(false)} onPointerDown={() => setPointerFocus(true)} onBlur={() => setPointerFocus(false)}
        {...stylex.props(styles.input)} />
      <button type="submit" aria-label={m.submit} title={m.submit} {...stylex.props(styles.submit)}><MagnifyingGlassIcon size={21} aria-hidden="true" /></button>
      <input type="hidden" name={region === 'CN' ? 'adlt' : 'safe'} value={region === 'CN' ? 'strict' : 'active'} />
    </form>
    <div role="group" aria-label={m.platforms} data-desktop-platforms onKeyDown={(event) => groupKeys(event)} {...stylex.props(styles.desktopPlatforms)}>
      {available.map((item) => <button key={item.id} type="button" aria-pressed={selected === item.id}
        onClick={() => choosePlatform(item.id)} {...stylex.props(controls.glass, styles.platform, selected === item.id && controls.selected)}>{item.name[locale]}</button>)}
    </div>
    <div ref={picker} data-mobile-platforms {...stylex.props(styles.mobilePlatforms)}
      onBlur={(event) => {
        if (!mobile.current?.matches && !event.relatedTarget) { setExpanded(false); focusCategory(); return }
        if (!event.currentTarget.contains(event.relatedTarget)) setExpanded(false)
      }} onKeyDown={(event) => {
        if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); setExpanded(false); trigger.current?.focus({ preventScroll: true }) }
        else groupKeys(event, true)
      }}>
      <div role="group" id={pickerId} aria-label={m.platforms} {...stylex.props(styles.grid(columns))}>
        {order.map((id, index) => {
          const item = available.find((item) => item.id === id)!
          const active = index === selectedSlot
          if (!active && !expanded) return null
          return <button key={active ? 'selected' : id} ref={active ? trigger : undefined} type="button" data-slot={index}
            aria-label={item.name[locale]} title={item.name[locale]} aria-pressed={active}
            aria-expanded={active ? expanded : undefined} aria-controls={active ? pickerId : undefined}
            disabled={active && available.length === 1}
            onClick={() => active ? setExpanded(!expanded) : choosePlatform(id, true)}
            {...stylex.props(controls.glass, styles.platform, styles.mobileOption, active && controls.selected, styles.slot(index % columns + 1, Math.floor(index / columns) + 1))}>
            <span {...stylex.props(styles.truncate)}>{item.name[locale]}</span>
            {active && available.length > 1 && (expanded ? <CaretUpIcon size={12} aria-hidden="true" /> : <CaretDownIcon size={12} aria-hidden="true" />)}
          </button>
        })}
      </div>
    </div>
  </div>
}

const styles = stylex.create({
  shell: { position: 'relative', display: 'flex', flexDirection: 'column', alignItems: 'center', width: '100%', maxWidth: 620, marginInline: 'auto', gap: 12 },
  categoryTrack: { width: '100%', maxWidth: 500, overflowX: 'auto', paddingBlock: 2, scrollbarWidth: 'none', '::-webkit-scrollbar': { display: 'none' } },
  categories: { display: 'flex', width: 'max-content', gap: { default: 12, '@media (max-width: 767px)': 8 }, marginInline: 'auto' },
  category: { flexShrink: 0, paddingBlock: 6, paddingInline: 12, minHeight: 36, fontSize: 14, lineHeight: '20px', whiteSpace: 'nowrap' },
  search: {
    display: 'flex', alignItems: 'center', width: '100%', maxWidth: 500, minHeight: 48, paddingInlineStart: 16, paddingInlineEnd: 6,
    borderWidth: 1, borderStyle: 'solid', borderRadius: tokens.radiusMedium, color: tokens.text,
    borderColor: { default: tokens.border, ':has(input:enabled):active': tokens.accent },
    backgroundColor: { default: tokens.surface, ':has(input:enabled):hover': tokens.surfaceHover },
    backdropFilter: 'blur(18px) saturate(1.08)', outlineStyle: 'none',
    opacity: { default: 1, ':has(input:disabled)': 0.5 },
  },
  keyboardFocus: {
    ':has(input:focus-visible)': { borderColor: 'transparent', outlineStyle: 'solid', outlineWidth: 2, outlineOffset: -2, outlineColor: tokens.accent },
  },
  input: {
    flexGrow: 1, minWidth: 0, width: '100%', padding: 0, borderWidth: 0, height: 46,
    outlineStyle: 'none', backgroundColor: 'transparent', color: tokens.text,
    cursor: { default: 'text', ':disabled': 'not-allowed' },
    fontFamily: 'inherit', fontSize: 16, lineHeight: '24px', '::placeholder': { color: tokens.text, opacity: 0.7 },
  },
  submit: {
    display: 'grid', placeItems: 'center', flexShrink: 0, width: 36, height: 36, padding: 0, borderWidth: 0, borderRadius: tokens.radiusSmall,
    backgroundColor: { default: 'transparent', ':enabled:hover': tokens.surfaceHover, ':enabled:active': tokens.surface }, color: tokens.text,
    cursor: { default: 'pointer', ':disabled': 'not-allowed' }, opacity: { default: 0.75, ':enabled:hover': 1, ':disabled': 0.4 },
    outlineStyle: { default: 'none', ':focus-visible': 'solid' }, outlineWidth: 2, outlineOffset: -2, outlineColor: tokens.accent,
  },
  desktopPlatforms: { display: { default: 'flex', '@media (max-width: 767px)': 'none' }, flexWrap: 'wrap', justifyContent: 'center', gap: 8, width: '100%' },
  platform: { paddingBlock: 6, paddingInline: 10, minHeight: 32, fontSize: 12, lineHeight: '16px', whiteSpace: 'nowrap' },
  mobilePlatforms: { display: { default: 'none', '@media (max-width: 767px)': 'block' }, width: '100%', maxWidth: 500, minHeight: 44 },
  grid: (columns: number) => ({ display: 'grid', gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gap: 6 }),
  slot: (column: number, row: number) => ({ gridColumn: column, gridRow: row }),
  mobileOption: { display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4, minWidth: 0, minHeight: 44, paddingInline: 8 },
  truncate: { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' },
})
