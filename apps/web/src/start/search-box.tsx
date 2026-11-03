import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr/CaretDown'
import { CaretUpIcon } from '@phosphor-icons/react/dist/ssr/CaretUp'
import * as stylex from '@stylexjs/stylex'
import { usePreferences } from '../preferences'
import { searchStyles as styles } from './search.stylex'
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
    // No suggestions exist: leave native Up/Down caret behavior untouched.
  }
  const defaultAction = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
  return <div ref={shell} data-start-search {...stylex.props(styles.shell)}>
    <div data-category-track {...stylex.props(styles.categoryTrack)}>
      <div role="group" aria-label={m.categories} data-categories onKeyDown={(event) => groupKeys(event)} {...stylex.props(styles.categories)}>
        {categories.map((id) => <button key={id} type="button" aria-pressed={category === id}
          onClick={(event) => chooseCategory(id, event.currentTarget)} {...stylex.props(styles.chip, styles.category, category === id && styles.selected)}>{m.category[id]}</button>)}
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
      <button type="submit" aria-label={m.submit} title={m.submit} {...stylex.props(styles.submit)}><MagnifyingGlassIcon size={20} weight="bold" aria-hidden="true" /></button>
      <input type="hidden" name={region === 'CN' ? 'adlt' : 'safe'} value={region === 'CN' ? 'strict' : 'active'} />
    </form>
    <div role="group" aria-label={m.platforms} data-desktop-platforms onKeyDown={(event) => groupKeys(event)} {...stylex.props(styles.desktopPlatforms)}>
      {available.map((item) => <button key={item.id} type="button" aria-pressed={selected === item.id}
        onClick={() => choosePlatform(item.id)} {...stylex.props(styles.chip, styles.platform, selected === item.id && styles.selected)}>{item.name[locale]}</button>)}
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
            {...stylex.props(styles.mobileOption, styles.slot(index % columns + 1, Math.floor(index / columns) + 1))}>
            <span {...stylex.props(styles.truncate)}>{item.name[locale]}</span>
            {active && available.length > 1 && (expanded ? <CaretUpIcon size={12} aria-hidden="true" /> : <CaretDownIcon size={12} aria-hidden="true" />)}
          </button>
        })}
      </div>
    </div>
  </div>
}
