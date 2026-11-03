import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr/CaretDown'
import { CaretLeftIcon } from '@phosphor-icons/react/dist/ssr/CaretLeft'
import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr/CaretRight'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import * as stylex from '@stylexjs/stylex'
import { usePreferences } from '../preferences'
import { darkSearch, systemSearch, searchStyles as styles } from './search.stylex'
import { useStartMessages } from './messages'
import { categories, defaultPlatform, platforms, searchURL, type Category } from './search-platforms'

type Panel = 'platforms' | 'suggestions' | null
type Suggestions = { query: string; items: string[] }

export function SearchBox() {
  const { region, locale, theme, themePreference } = usePreferences()
  const m = useStartMessages()
  const [category, setCategory] = useState<Category>('search')
  const [selected, setSelected] = useState(defaultPlatform('search', region).id)
  const [keyword, setKeyword] = useState('')
  const [panel, setPanel] = useState<Panel>(null)
  const [mountedPanel, setMountedPanel] = useState<Panel>(null)
  const [visible, setVisible] = useState(false)
  const [suggestions, setSuggestions] = useState<Suggestions>({ query: '', items: [] })
  const [active, setActive] = useState(-1)
  const [pointerFocus, setPointerFocus] = useState(false)
  const composing = useRef(false)
  const shell = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const popup = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pending = useRef<AbortController | null>(null)
  const sequence = useRef(0)
  const panelRef = useRef<Panel>(null)
  const scheduled = useRef('')
  const focusPicker = useRef(false)
  const categoryTrack = useRef<HTMLDivElement>(null)
  const skipFocusRequest = useRef(false)
  const [categoryEdges, setCategoryEdges] = useState({ left: false, right: false })
  const panelId = useId()
  const available = platforms[category]
  const target = available.find((item) => item.id === selected)!

  useEffect(() => {
    const track = categoryTrack.current!
    let active = true
    const measure = () => {
      if (!active) return
      const focused = track.contains(document.activeElement) ? document.activeElement : track.querySelector('[aria-pressed="true"]')
      if (focused instanceof HTMLButtonElement) revealCategory(focused)
      updateCategoryEdges()
    }
    const observer = new ResizeObserver(measure)
    observer.observe(track); observer.observe(track.firstElementChild!)
    void document.fonts.ready.then(measure)
    return () => { active = false; observer.disconnect() }
  }, [])

  // One physical panel prevents overlapping menus, including during exit.
  // Keep its last content mounted for the complete fade-out (no empty shell).
  useEffect(() => {
    let frame = 0, nextFrame = 0
    let exit: ReturnType<typeof setTimeout> | undefined
    if (panel) {
      setMountedPanel(panel)
      frame = requestAnimationFrame(() => { nextFrame = requestAnimationFrame(() => setVisible(true)) })
    } else {
      setVisible(false)
      exit = setTimeout(() => setMountedPanel(null), matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 420)
    }
    return () => { cancelAnimationFrame(frame); cancelAnimationFrame(nextFrame); clearTimeout(exit) }
  }, [panel])
  useEffect(() => {
    const query = matchMedia('(max-width: 767px)')
    const breakpoint = () => {
      if (!query.matches && panelRef.current === 'platforms') {
        closePanel()
        input.current?.focus({ preventScroll: true })
      }
    }
    const outside = (event: PointerEvent) => {
      if (!shell.current?.contains(event.target as Node)) { cancelRequest(); closePanel() }
    }
    query.addEventListener('change', breakpoint)
    document.addEventListener('pointerdown', outside)
    return () => { query.removeEventListener('change', breakpoint); document.removeEventListener('pointerdown', outside); cancelRequest() }
  }, [])
  useEffect(() => {
    if (panel === 'platforms' && mountedPanel === 'platforms' && focusPicker.current) {
      popup.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')?.focus({ preventScroll: true })
    }
  }, [panel, mountedPanel])
  useEffect(() => {
    if (panel !== 'suggestions') return
    const list = popup.current, row = list?.querySelector<HTMLElement>(`[data-index="${active}"]`)
    if (list && row) {
      if (row.offsetTop < list.scrollTop) list.scrollTop = row.offsetTop
      else if (row.offsetTop + row.offsetHeight > list.scrollTop + list.clientHeight) list.scrollTop = row.offsetTop + row.offsetHeight - list.clientHeight
    }
  }, [active, panel])

  function showPanel(next: Panel) { panelRef.current = next; setPanel(next) }
  function updateCategoryEdges() {
    const track = categoryTrack.current
    if (!track) return
    const next = { left: track.scrollLeft > 1, right: track.scrollLeft + track.clientWidth < track.scrollWidth - 1 }
    setCategoryEdges((old) => old.left === next.left && old.right === next.right ? old : next)
  }
  function revealCategory(button: HTMLButtonElement) {
    const track = categoryTrack.current!
    const gutter = track.scrollWidth > track.clientWidth ? 20 : 0
    const item = button.getBoundingClientRect(), bounds = track.getBoundingClientRect()
    if (item.left < bounds.left + gutter) track.scrollLeft -= bounds.left + gutter - item.left
    else if (item.right > bounds.right - gutter) track.scrollLeft += item.right - bounds.right + gutter
  }
  function closePanel() { showPanel(null); setActive(-1) }
  function cancelRequest() {
    sequence.current++
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
    pending.current?.abort(); pending.current = null
    scheduled.current = ''
  }
  function schedule(value: string) {
    const query = value.trim()
    // compositionend can be followed by an identical input event.
    if (scheduled.current === query && query) return
    cancelRequest(); closePanel()
    if (category !== 'search' || composing.current || !query || [...query].length > 128) return
    scheduled.current = query
    const id = sequence.current
    timer.current = setTimeout(async () => {
      const controller = new AbortController()
      pending.current = controller
      try {
        const response = await fetch(`/api/v1/start/search/suggestions?q=${encodeURIComponent(query)}`, {
          signal: AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]), credentials: 'omit',
        })
        // Development's existing API proxy must enforce the same regional
        // boundary as the production Web relay, before publishing any phrases.
        if (!response.ok || response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') throw new Error('unavailable')
        const body = await response.json() as { suggestions: string[] }
        if (id !== sequence.current || controller.signal.aborted || document.activeElement !== input.current || composing.current) return
        const items = Array.isArray(body.suggestions) ? body.suggestions.filter((item) => typeof item === 'string' && item.trim() && [...item].length <= 128).slice(0, 8) : []
        if (!items.length) { closePanel(); return }
        setSuggestions({ query, items }); setActive(0); showPanel('suggestions')
      } catch { if (id === sequence.current) closePanel() }
      finally { if (id === sequence.current) { pending.current = null; scheduled.current = ''; timer.current = null } }
    }, 600)
  }
  function chooseCategory(next: Category, button: HTMLButtonElement) {
    cancelRequest(); closePanel()
    setCategory(next); setSelected(defaultPlatform(next, region).id)
    revealCategory(button)
  }
  function choosePlatform(id: string, fromMobile = false) {
    setSelected(id)
    if (fromMobile) { cancelRequest(); closePanel(); trigger.current?.focus({ preventScroll: true }) }
  }
  function groupKeys(event: KeyboardEvent<HTMLElement>) {
    if (event.nativeEvent.isComposing) return
    const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
    const step = event.key === 'ArrowRight' || event.key === 'ArrowDown' ? 1 : event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 0
    if (step || event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      buttons[event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + step + buttons.length) % buttons.length]?.focus({ preventScroll: true })
    }
  }
  function submit(value: string) {
    if (composing.current || !value.trim()) return
    cancelRequest(); closePanel()
    window.open(searchURL(target, value, locale), '_blank', 'noopener,noreferrer')
  }
  function inputKeys(event: KeyboardEvent<HTMLInputElement>) {
    if (composing.current || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) {
      if (event.key === 'Enter') event.preventDefault()
      return
    }
    if (event.key === 'Escape') { event.preventDefault(); cancelRequest(); closePanel() }
    if (panel !== 'suggestions' || !suggestions.items.length) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      setActive((current) => (current + (event.key === 'ArrowDown' ? 1 : -1) + suggestions.items.length) % suggestions.items.length)
    } else if (event.key === 'Enter' && active >= 0) {
      event.preventDefault(); submit(suggestions.items[active])
    }
  }
  function highlighted(phrase: string) {
    const index = phrase.toLocaleLowerCase().indexOf(suggestions.query.toLocaleLowerCase())
    if (index < 0) return phrase
    const end = index + suggestions.query.length
    return <span>{phrase.slice(0, index)}<mark {...stylex.props(styles.highlight)}>{phrase.slice(index, end)}</mark>{phrase.slice(end)}</span>
  }
  const defaultAction = region === 'CN' ? 'https://www.bing.com/search' : 'https://www.google.com/search'
  return <div ref={shell} data-start-search {...stylex.props(styles.shell, themePreference === 'system' ? systemSearch : theme === 'dark' && darkSearch)}
    onBlur={(event) => {
      if (!event.currentTarget.contains(event.relatedTarget)) { cancelRequest(); closePanel() }
    }}>
    <div {...stylex.props(styles.categoryViewport)}>
    <div ref={categoryTrack} data-category-track onScroll={updateCategoryEdges} {...stylex.props(styles.categoryTrack, categoryEdges.left && styles.fadeLeft, categoryEdges.right && styles.fadeRight, categoryEdges.left && categoryEdges.right && styles.fadeBoth)}>
      <div role="group" aria-label={m.categories} aria-description={categoryEdges.left || categoryEdges.right ? m.moreCategories : undefined} data-categories onKeyDown={groupKeys}
        onFocusCapture={(event) => { if (event.target instanceof HTMLButtonElement) revealCategory(event.target) }} {...stylex.props(styles.categories)}>
        {categories.map((id) => <button key={id} type="button" aria-pressed={category === id}
          onClick={(event) => chooseCategory(id, event.currentTarget)} {...stylex.props(styles.chip, styles.category, category === id && styles.selected)}>{m.category[id]}</button>)}
      </div>
    </div>
    {categoryEdges.left && <span aria-hidden="true" title={m.moreCategories} data-category-edge="left" {...stylex.props(styles.edge, styles.edgeLeft)}><CaretLeftIcon size={12} /></span>}
    {categoryEdges.right && <span aria-hidden="true" title={m.moreCategories} data-category-edge="right" {...stylex.props(styles.edge, styles.edgeRight)}><CaretRightIcon size={12} /></span>}
    </div>
    <div {...stylex.props(styles.anchor)}>
    <form role="search" aria-label={m.search} method="get" action={defaultAction} target="_blank" rel="noopener noreferrer" acceptCharset="UTF-8"
      onSubmit={(event) => { event.preventDefault(); submit(panel === 'suggestions' && active >= 0 ? suggestions.items[active] : keyword) }}
      {...stylex.props(styles.search, !pointerFocus && styles.keyboardFocus)}>
      <button ref={trigger} type="button" data-mobile-platform-trigger aria-label={`${m.platforms}: ${target.name[locale]}`}
        aria-haspopup="listbox" aria-expanded={panel === 'platforms'} aria-controls={panel === 'platforms' ? panelId : undefined} disabled={available.length === 1}
        onClick={(event) => { cancelRequest(); focusPicker.current = event.detail === 0; showPanel(panel === 'platforms' ? null : 'platforms') }}
        onKeyDown={(event) => {
          if (event.key === 'Escape') { event.preventDefault(); cancelRequest(); closePanel() }
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault(); cancelRequest(); focusPicker.current = true
            if (panel === 'platforms') popup.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')?.focus({ preventScroll: true })
            else showPanel('platforms')
          }
        }}
        {...stylex.props(styles.mobileTrigger)}><span {...stylex.props(styles.truncate)}>{target.name[locale]}</span><CaretDownIcon size={12} aria-hidden="true" {...stylex.props(styles.caret)} /></button>
      <input ref={input} type="search" name="q" role={category === 'search' ? 'combobox' : undefined} aria-label={m.search} placeholder={m.search} required pattern={String.raw`.*\S.*`}
        aria-autocomplete={category === 'search' ? 'list' : undefined} aria-expanded={category === 'search' ? panel === 'suggestions' : undefined}
        aria-controls={panel === 'suggestions' ? panelId : undefined} aria-activedescendant={panel === 'suggestions' && active >= 0 ? `${panelId}-${active}` : undefined}
        value={keyword} autoComplete="off" autoCapitalize="off" spellCheck={false} enterKeyHint="search"
        onChange={(event) => { setKeyword(event.target.value); schedule(event.target.value) }} onKeyDown={inputKeys}
        onCompositionStart={() => { composing.current = true; cancelRequest(); closePanel() }}
        onCompositionEnd={(event) => { composing.current = false; setKeyword(event.currentTarget.value); schedule(event.currentTarget.value) }}
        onFocus={() => { if (panelRef.current === 'platforms') closePanel(); if (!skipFocusRequest.current) schedule(keyword) }}
        onPointerDown={() => setPointerFocus(true)} onBlur={(event) => {
          setPointerFocus(false); cancelRequest()
          if (panelRef.current === 'suggestions' && !popup.current?.contains(event.relatedTarget)) closePanel()
        }}
        {...stylex.props(styles.input, !!keyword && styles.inputWithClear)} />
      {!!keyword && <button type="button" aria-label={m.clear} title={m.clear} data-search-clear
        onPointerDown={(event) => event.preventDefault()} onClick={() => {
          cancelRequest(); closePanel(); composing.current = false; setKeyword('')
          skipFocusRequest.current = true; input.current?.focus({ preventScroll: true }); skipFocusRequest.current = false
        }} {...stylex.props(styles.submit, styles.clear)}><XIcon size={18} aria-hidden="true" /></button>}
      <button type="submit" aria-label={m.submit} title={m.submit} {...stylex.props(styles.submit)}><MagnifyingGlassIcon size={20} weight="bold" aria-hidden="true" /></button>
      <input type="hidden" name={region === 'CN' ? 'adlt' : 'safe'} value={region === 'CN' ? 'strict' : 'active'} />
    </form>
      {mountedPanel && <div ref={popup} id={panelId} role="listbox" aria-label={mountedPanel === 'platforms' ? m.platforms : m.suggestions}
        data-search-panel={mountedPanel} data-open={visible && panel === mountedPanel} aria-hidden={!panel} inert={!panel}
        onKeyDown={(event) => {
          if (event.key === 'Escape') { event.preventDefault(); cancelRequest(); closePanel(); (mountedPanel === 'platforms' ? trigger : input).current?.focus({ preventScroll: true }) }
          else if (mountedPanel === 'platforms') groupKeys(event)
        }} {...stylex.props(styles.panel)}>
        {mountedPanel === 'platforms' ? available.map((item) => <button key={item.id} type="button" role="option" aria-selected={selected === item.id}
          onClick={() => choosePlatform(item.id, true)} {...stylex.props(styles.option, styles.optionBase, selected === item.id && styles.selected)}>{item.name[locale]}</button>)
          : suggestions.items.map((phrase, index) => <button key={phrase} id={`${panelId}-${index}`} data-index={index} type="button" role="option" tabIndex={-1}
            aria-selected={active === index} onPointerMove={() => setActive(index)} onPointerDown={(event) => event.preventDefault()}
            onClick={() => submit(phrase)} {...stylex.props(styles.option, styles.optionBase, active === index && styles.selected)}>{highlighted(phrase)}</button>)}
      </div>}
    </div>
    <div role="group" aria-label={m.platforms} data-desktop-platforms onKeyDown={groupKeys} {...stylex.props(styles.desktopPlatforms)}>
      {available.map((item) => <button key={item.id} type="button" aria-pressed={selected === item.id}
        onClick={() => choosePlatform(item.id)} {...stylex.props(styles.chip, styles.platform, selected === item.id && styles.selected)}>{item.name[locale]}</button>)}
    </div>
  </div>
}
