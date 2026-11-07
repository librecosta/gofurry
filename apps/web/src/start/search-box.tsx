import { cx } from '../styles/classes'
import { useEffect, useLayoutEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type KeyboardEventHandler, type ReactNode, type Ref } from 'react'
import { AnimatePresence, motion, useIsPresent } from 'motion/react'
import { useMotionTiming } from '../motion'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr/CaretDown'
import { CaretLeftIcon } from '@phosphor-icons/react/dist/ssr/CaretLeft'
import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr/CaretRight'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { QuickAccess } from './quick-access'
import { useStartSurface } from './start-material'
import { usePreferences } from '../preferences'
import styles from './search.module.scss'
import { useStartMessages } from './messages'
import { categories, availablePlatforms, resolveSelection, searchURL, type Category } from './search-platforms'

type Panel = 'platforms' | 'suggestions' | null
type Suggestions = { query: string; items: string[] }

export function SearchBox() {
  const hoverTransition = useMotionTiming(0.3)
  const validationTransition = useMotionTiming(0.18)
  const { region, locale, quickAccess, contentPreference, showFurrySearch } = usePreferences()
  const m = useStartMessages()
  const [mobile, setMobile] = useState<boolean | null>(null)
  const [platformWidth, setPlatformWidth] = useState<number>()
  const platformName = useRef<HTMLSpanElement>(null)
  const platformMeasured = useRef(false)
  const [tailHeight, setTailHeight] = useState<number>()
  const tailTransition = useMotionTiming()
  const [selection, setSelection] = useState<{ category: Category; id: string }>({ category: 'search', id: 'bing' })
  const [keyword, setKeyword] = useState('')
  const [emptySearch, setEmptySearch] = useState(false)
  const [panel, setPanel] = useState<Panel>(null)
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
  const validationId = useId()
  const catalog = useMemo(() => availablePlatforms(region, contentPreference, showFurrySearch), [region, contentPreference, showFurrySearch])
  // Resolve synchronously: no render or submit can use a hidden/missing target.
  const { category, target, choices: available } = resolveSelection(catalog, selection.category, selection.id)
  const selected = target.id
  useEffect(() => {
    setSelection(previous => {
      const next = resolveSelection(catalog, previous.category, previous.id)
      return previous.category === next.category && previous.id === next.target.id ? previous : { category: next.category, id: next.target.id }
    })
    cancelRequest(); closePanel()
  }, [catalog])

  useLayoutEffect(() => {
    let active = true
    const measure = () => {
      if (!active || !platformName.current) return
      const name = platformName.current, inner = name.parentElement!
      const range = document.createRange(); range.selectNodeContents(name)
      const css = getComputedStyle(inner)
      const width = range.getBoundingClientRect().width + parseFloat(css.paddingLeft) + parseFloat(css.paddingRight) + parseFloat(css.columnGap) + inner.querySelector('svg')!.getBoundingClientRect().width
      // Establish Motion's numeric starting size before the first hydrated paint.
      // SSR still uses CSS for the remembered preference and mobile breakpoint.
      if (!platformMeasured.current) {
        trigger.current?.style.setProperty('--platform-size', `${quickAccess || matchMedia('(max-width: 767px)').matches ? width : 0}px`)
        platformMeasured.current = true
      }
      setPlatformWidth(width)
    }
    measure(); void document.fonts.ready.then(measure)
    return () => { active = false }
  }, [target.name[locale]])

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
  }, [showFurrySearch])

  useEffect(() => {
    const query = matchMedia('(max-width: 767px)')
    const breakpoint = () => {
      setMobile(query.matches)
      if (!query.matches && !quickAccess && panelRef.current === 'platforms') {
        closePanel()
        input.current?.focus({ preventScroll: true })
      }
    }
    const outside = (event: PointerEvent) => {
      if (!shell.current?.contains(event.target as Node)) { cancelRequest(); closePanel() }
    }
    breakpoint()
    query.addEventListener('change', breakpoint)
    document.addEventListener('pointerdown', outside)
    return () => { query.removeEventListener('change', breakpoint); document.removeEventListener('pointerdown', outside); cancelRequest() }
  }, [quickAccess])
  useEffect(() => {
    if (panel === 'platforms' && focusPicker.current) {
      popup.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')?.focus({ preventScroll: true })
    }
  }, [panel])
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
    const resolved = resolveSelection(catalog, next, '')
    setSelection({ category: resolved.category, id: resolved.target.id })
    revealCategory(button)
  }
  function choosePlatform(id: string, fromMobile = false) {
    if (!available.some(item => item.id === id)) return
    setSelection({ category, id })
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
  function reportEmptySearch() {
    cancelRequest(); closePanel()
    setEmptySearch(true)
    input.current?.focus({ preventScroll: true })
  }
  function submit(value: string) {
    if (composing.current) return
    if (!value.trim()) {
      reportEmptySearch()
      return
    }
    setEmptySearch(false)
    cancelRequest(); closePanel()
    window.open(searchURL(target, value, locale), '_blank', 'noopener,noreferrer')
  }
  function inputKeys(event: KeyboardEvent<HTMLInputElement>) {
    if (composing.current || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) {
      if (event.key === 'Enter') event.preventDefault()
      return
    }
    if (event.key === 'Escape') { event.preventDefault(); setEmptySearch(false); cancelRequest(); closePanel() }
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
    return <span>{phrase.slice(0, index)}<mark className={styles.highlight}>{phrase.slice(index, end)}</mark>{phrase.slice(end)}</span>
  }
  const defaultAction = 'https://www.bing.com/search'
  const searchGlass = useStartSurface('search')
  return <div ref={shell} data-start-search data-quick-enabled={quickAccess} className={cx(styles.shell, quickAccess && styles.quickEnabled)}
    onBlur={(event) => {
      if (!event.currentTarget.contains(event.relatedTarget)) { cancelRequest(); closePanel() }
    }}>
    <div className={styles.categoryViewport}>
    <div ref={categoryTrack} data-category-track onScroll={updateCategoryEdges} className={cx(styles.categoryTrack, categoryEdges.left && styles.fadeLeft, categoryEdges.right && styles.fadeRight, categoryEdges.left && categoryEdges.right && styles.fadeBoth)}>
      <div role="group" aria-label={m.categories} aria-description={categoryEdges.left || categoryEdges.right ? m.moreCategories : undefined} data-categories onKeyDown={groupKeys}
        onFocusCapture={(event) => { if (event.target instanceof HTMLButtonElement) revealCategory(event.target) }} className={styles.categories}>
        {categories.filter(id => catalog.some(item => item.category === id)).map(id => <SearchCategoryButton key={id}
          label={m.category[id]} selected={category === id} onSelect={button => chooseCategory(id, button)} />)}
      </div>
    </div>
    {categoryEdges.left && <span aria-hidden="true" data-category-edge="left" className={cx(styles.edge, styles.edgeLeft)}><CaretLeftIcon size={12} /></span>}
    {categoryEdges.right && <span aria-hidden="true" data-category-edge="right" className={cx(styles.edge, styles.edgeRight)}><CaretRightIcon size={12} /></span>}
    </div>
    <div className={styles.anchor}>
    <form role="search" aria-label={m.search} method="get" action={defaultAction} target="_blank" rel="noopener noreferrer" acceptCharset="UTF-8"
      onSubmit={(event) => { event.preventDefault(); submit(panel === 'suggestions' && active >= 0 ? suggestions.items[active] : keyword) }}
      data-start-transparent={searchGlass.props['data-start-transparent']}
      className={cx(styles.search, !pointerFocus && styles.keyboardFocus, searchGlass.ready && styles.liquidSearch, searchGlass.transparent && searchGlass.className)}>
      {searchGlass.enabled && <span aria-hidden="true" {...searchGlass.props} className={cx(styles.searchSurface, searchGlass.className)} />}
      {searchGlass.binding}
      <motion.button ref={trigger} type="button" data-mobile-platform-trigger initial={false}
        animate={mobile === null || platformWidth === undefined ? undefined : { '--platform-size': `${quickAccess || mobile ? platformWidth : 0}px`, opacity: quickAccess || mobile ? 1 : 0 }} transition={tailTransition} aria-label={`${m.platforms}: ${target.name[locale]}`}
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
        className={styles.mobileTrigger}><span className={styles.inlinePlatform}><span ref={platformName} className={styles.truncate}>{target.name[locale]}</span><CaretDownIcon size={12} aria-hidden="true" className={styles.caret} /></span></motion.button>
      <input ref={input} type="search" name="q" role={category === 'search' ? 'combobox' : undefined} aria-label={m.search} placeholder={m.search} required pattern={String.raw`.*\S.*`}
        aria-invalid={emptySearch || undefined} aria-describedby={emptySearch ? validationId : undefined}
        aria-autocomplete={category === 'search' ? 'list' : undefined} aria-expanded={category === 'search' ? panel === 'suggestions' : undefined}
        aria-controls={panel === 'suggestions' ? panelId : undefined} aria-activedescendant={panel === 'suggestions' && active >= 0 ? `${panelId}-${active}` : undefined}
        value={keyword} autoComplete="off" autoCapitalize="off" spellCheck={false} enterKeyHint="search"
        onInvalid={(event) => {
          // Cancel the native bubble only once this handler is attached.
          event.preventDefault()
          if (!composing.current) reportEmptySearch()
        }}
        onChange={(event) => { setEmptySearch(false); setKeyword(event.target.value); schedule(event.target.value) }} onKeyDown={inputKeys}
        onCompositionStart={() => { composing.current = true; setEmptySearch(false); cancelRequest(); closePanel() }}
        onCompositionEnd={(event) => { composing.current = false; setKeyword(event.currentTarget.value); schedule(event.currentTarget.value) }}
        onFocus={() => { if (panelRef.current === 'platforms') closePanel(); if (!skipFocusRequest.current) schedule(keyword) }}
        onPointerDown={() => setPointerFocus(true)} onBlur={(event) => {
          setEmptySearch(false); setPointerFocus(false); cancelRequest()
          if (panelRef.current === 'suggestions' && !popup.current?.contains(event.relatedTarget)) closePanel()
        }}
        className={cx(styles.input, !!keyword && styles.inputWithClear)} />
      {!!keyword && <motion.button type="button" aria-label={m.clear} data-search-clear initial={false} animate={{ opacity: 0.72 }} whileHover={{ opacity: 0.96 }} whileTap={{ opacity: 1 }} transition={hoverTransition}
        onPointerDown={(event) => event.preventDefault()} onClick={() => {
          cancelRequest(); closePanel(); composing.current = false; setEmptySearch(false); setKeyword('')
          skipFocusRequest.current = true; input.current?.focus({ preventScroll: true }); skipFocusRequest.current = false
        }} className={cx(styles.submit, styles.clear)}><XIcon size={18} aria-hidden="true" /></motion.button>}
      <motion.button type="submit" aria-label={m.submit} className={styles.submit} initial={false} animate={{ opacity: 0.72 }} whileHover={{ opacity: 0.96 }} whileTap={{ opacity: 1 }} transition={hoverTransition}><MagnifyingGlassIcon size={20} weight="bold" aria-hidden="true" /></motion.button>
      <input type="hidden" name="adlt" value="strict" />
    </form>
      <div id={validationId} role="status" className={styles.validation}>
        <AnimatePresence>{emptySearch && <motion.span className={styles.validationMessage}
          initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={validationTransition}>
          {m.emptySearch}
        </motion.span>}</AnimatePresence>
      </div>
      <AnimatePresence>{panel && <SearchPanel key="search-panel" ref={popup} id={panelId} label={panel === 'platforms' ? m.platforms : m.suggestions} kind={panel}
        onKeyDown={(event) => {
          if (event.key === 'Escape') { event.preventDefault(); cancelRequest(); closePanel(); (panel === 'platforms' ? trigger : input).current?.focus({ preventScroll: true }) }
          else if (panel === 'platforms') groupKeys(event)
        }}>
        {panel === 'platforms' ? available.map((item) => <button key={item.id} type="button" role="option" aria-selected={selected === item.id}
          onClick={() => choosePlatform(item.id, true)} className={cx(styles.option, styles.optionBase, selected === item.id && styles.selected)}>{item.name[locale]}</button>)
          : suggestions.items.map((phrase, index) => <button key={phrase} id={`${panelId}-${index}`} data-index={index} type="button" role="option" tabIndex={-1}
            aria-selected={active === index} onPointerMove={() => setActive(index)} onPointerDown={(event) => event.preventDefault()}
            onClick={() => submit(phrase)} className={cx(styles.option, styles.optionBase, active === index && styles.selected)}>{highlighted(phrase)}</button>)}
      </SearchPanel>}</AnimatePresence>
    </div>
    <motion.div data-search-tail className={styles.tail} initial={false} animate={{ height: tailHeight ?? 'auto' }} transition={tailTransition}><AnimatePresence initial={false}>
    {quickAccess ? <SearchTailView key="quick" onMeasure={setTailHeight}><QuickAccess /></SearchTailView> : <SearchTailView key="platforms" onMeasure={setTailHeight}><div role="group" aria-label={m.platforms} data-desktop-platforms onKeyDown={groupKeys} className={styles.desktopPlatforms}>
      {available.map((item) => <button key={item.id} type="button" aria-pressed={selected === item.id}
        onClick={() => choosePlatform(item.id)} className={cx(styles.chip, styles.platform, selected === item.id && styles.selected)}>{item.name[locale]}</button>)}
    </div></SearchTailView>}
    </AnimatePresence></motion.div>
  </div>
}

function SearchTailView({ children, onMeasure }: { children: ReactNode; onMeasure: (height: number) => void }) {
  const present = useIsPresent(), transition = useMotionTiming()
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    if (!present) return
    const element = ref.current!, measure = () => onMeasure(element.getBoundingClientRect().height)
    measure()
    const observer = new ResizeObserver(measure); observer.observe(element)
    return () => observer.disconnect()
  }, [present, onMeasure])
  return <motion.div ref={ref} className={styles.tailView} inert={!present} aria-hidden={!present || undefined}
    initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition}>{children}</motion.div>
}

// One physical panel is reused for both lists. Presence retains its last
// content during exit, while inert prevents a closing list receiving clicks.
function SearchCategoryButton({ label, selected, onSelect }: { label: string; selected: boolean; onSelect: (button: HTMLButtonElement) => void }) {
  const glass = useStartSurface('category')
  return <>
    <button {...glass.props} type="button" aria-pressed={selected} onClick={event => onSelect(event.currentTarget)}
      className={cx(styles.chip, styles.category, selected && styles.selected, glass.className)}>{label}</button>
    {glass.binding}
  </>
}

function SearchPanel({ ref, id, label, kind, onKeyDown, children }: { ref: Ref<HTMLDivElement>; id: string; label: string; kind: Exclude<Panel, null>; onKeyDown: KeyboardEventHandler<HTMLDivElement>; children: ReactNode }) {
  const present = useIsPresent()
  const transition = useMotionTiming()
  // Both lists fade without moving glyphs across fractional device pixels.
  return <motion.div ref={ref} id={id} role="listbox" aria-label={label} data-search-panel={kind} data-open={present} aria-hidden={!present} inert={!present}
    onKeyDown={onKeyDown} className={styles.panel} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition}>{children}</motion.div>
}
