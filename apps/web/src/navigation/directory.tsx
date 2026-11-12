import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { Dialog } from '@base-ui/react/dialog'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr/MagnifyingGlass'
import { SlidersHorizontalIcon } from '@phosphor-icons/react/dist/ssr/SlidersHorizontal'
import { XIcon } from '@phosphor-icons/react/dist/ssr/X'
import { categoryCodes } from './contracts'
import { apiQuery, criteriaParams, criteriaQuery, decodeDirectory, emptyCriteria, normalizeCriteria, normalizeKeyword, sorts, type Criteria, type Directory, type Sort, type Term } from './data'
import { languageLabel, SiteCard } from './cards'
import { categories, copy } from './messages'
import type { Page } from './load'
import { SitesFrame } from './pages'
import styles from './sites.module.scss'

type Dimension = 'category' | 'tags' | 'language'
type Option = { code: string; label: string }
function toggle(criteria: Criteria, dimension: Dimension, code: string): Criteria {
  const values = criteria[dimension]
  return { ...criteria, [dimension]: values.includes(code) ? values.filter(value => value !== code) : [...values, code].sort() }
}

function FilterDialog({ criteria, options, locale, apply }: { criteria: Criteria; options: Record<Dimension, Option[]>;
  locale: Page['locale']; apply: (draft: Criteria) => void }) {
  const m = copy[locale]
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(criteria)
  const trigger = useRef<HTMLButtonElement>(null)
  const dimensions = [
    ['category', m.categories], ['tags', m.tags], ['language', m.languages],
  ] as const
  const active = criteria.category.length + criteria.tags.length + criteria.language.length
  return <Dialog.Root open={open} onOpenChange={value => { if (value) setDraft(criteria); setOpen(value) }}>
    <Dialog.Trigger ref={trigger} className={styles.outline}><SlidersHorizontalIcon size={17} aria-hidden="true" />{m.filters}{active ? ` (${active})` : ''}</Dialog.Trigger>
    <Dialog.Portal>
      <Dialog.Backdrop className={styles.backdrop} />
      <Dialog.Popup className={styles.dialog} finalFocus={trigger}>
        <Dialog.Title>{m.filters}</Dialog.Title>
        <Dialog.Description>{m.languageHint}</Dialog.Description>
        {dimensions.map(([key, label]) => options[key].length > 0 && <fieldset key={key} className={styles.filterGroup}>
          <legend>{label}</legend>
          <div>{options[key].map(option => <label key={option.code}><input type="checkbox" checked={draft[key].includes(option.code)}
            disabled={draft[key].length >= 8 && !draft[key].includes(option.code)}
            onChange={() => setDraft(value => toggle(value, key, option.code))} />{option.label}</label>)}</div>
        </fieldset>)}
        <label className={styles.sheetSort}>{m.sort}
          <select className={styles.outline} value={draft.sort ?? (draft.q ? 'relevance' : 'published_desc')}
            onChange={event => setDraft(value => ({ ...value, sort: event.target.value as Sort }))}>
            {sorts.filter(sort => sort !== 'relevance' || draft.q).map(sort => <option key={sort} value={sort}>{m[sort]}</option>)}
          </select>
        </label>
        <div className={styles.dialogActions}>
          <Dialog.Close className={styles.outline}>{m.cancel}</Dialog.Close>
          <button type="button" className={styles.action} onClick={() => { setOpen(false); apply(draft) }}>{m.apply}</button>
        </div>
      </Dialog.Popup>
    </Dialog.Portal>
  </Dialog.Root>
}

type PageError = 'stale' | 'unavailable' | null

export function DirectoryPage({ page, data }: { page: Page; data: Directory }) {
  const { locale, region, path } = page
  const criteria = page.criteria ?? emptyCriteria
  const key = criteriaQuery(criteria)
  const m = copy[locale]
  const navigate = useNavigate()
  const router = useRouter()
  const changing = useRouterState({ select: state => state.isLoading })
  const locationKey = useRouterState({ select: state => state.location.href })
  const [query, setQuery] = useState(criteria.q)
  const [loading, setLoading] = useState(false)
  const [issue, setIssue] = useState<PageError>(null)
  const [extra, setExtra] = useState<{ scope: string; source: Directory; rows: Directory['items']; next?: string }>()
  const debounce = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const pending = useRef<AbortController | undefined>(undefined)
  const composing = useRef(false)
  const submitted = useRef<string | undefined>(undefined)
  const searchInput = useRef<HTMLInputElement>(null)
  const resultHeading = useRef<HTMLParagraphElement>(null)
  const scope = region + path + key
  const accumulated = extra?.scope === scope && extra.source === data ? extra : undefined
  const results = accumulated?.rows ?? data.items
  const next = accumulated ? accumulated.next : data.next
  const livePage = useRef(data)
  const liveLocation = useRef(locationKey)
  livePage.current = data
  liveLocation.current = locationKey

  useEffect(() => {
    if (submitted.current !== key) {
      clearTimeout(debounce.current)
      setQuery(criteria.q)
    }
    submitted.current = undefined
  }, [key, criteria.q])
  useEffect(() => {
    setIssue(null)
    setLoading(false)
    setExtra(undefined)
    return () => { pending.current?.abort(); pending.current = undefined }
  }, [data, locationKey])
  useEffect(() => () => clearTimeout(debounce.current), [])

  function cancelPage() {
    pending.current?.abort()
    pending.current = undefined
    setLoading(false)
    setIssue(null)
  }

  function commit(next: Criteria) {
    clearTimeout(debounce.current)
    const normalized = normalizeCriteria(next)
    const signature = criteriaQuery(normalized)
    setQuery(normalized.q)
    if (signature === key && !changing) return
    cancelPage()
    submitted.current = signature
    void navigate({ to: path, search: criteriaParams(normalized), resetScroll: false })
  }

  function search(value: string) {
    setQuery(value)
    cancelPage()
    clearTimeout(debounce.current)
    if (!composing.current) {
      debounce.current = setTimeout(() => commit({ ...criteria, q: value }), 300)
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!composing.current) commit({ ...criteria, q: query })
  }

  async function loadMore() {
    if (!next || pending.current || changing) return
    const restoreFocus = document.activeElement?.hasAttribute('data-load-more')
    const controller = new AbortController()
    pending.current = controller
    const current = () => !controller.signal.aborted && pending.current === controller && livePage.current === data && liveLocation.current === locationKey
    setLoading(true)
    setIssue(null)
    try {
      const response = await fetch('/api/v1/sites/directory?' + apiQuery(criteria, locale, next), {
        signal: AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]), redirect: 'error', credentials: 'omit',
      })
      if (!current()) return
      if (response.status === 400) { setIssue('stale'); return }
      if (!response.ok || response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') throw new Error()
      const page = decodeDirectory(await response.json())
      if (!current()) return
      const seen = new Set(results.map(item => item.public_id))
      const additions = page.items.filter(item => {
        if (seen.has(item.public_id)) return false
        seen.add(item.public_id)
        return true
      })
      const after = page.next === next ? undefined : page.next
      setExtra({ scope, source: data, rows: [...results, ...additions], next: after })
      if (!after && restoreFocus) resultHeading.current?.focus()
    } catch {
      if (current()) setIssue('unavailable')
    } finally {
      if (current()) { pending.current = undefined; setLoading(false) }
    }
  }

  async function refresh() {
    cancelPage()
    setExtra(undefined)
    setLoading(true)
    try {
      await router.invalidate()
      resultHeading.current?.focus()
    } finally { setLoading(false) }
  }

  const options = {
    category: categoryCodes.map(code => ({ code, label: categories[code][locale] })),
    tags: data.tag_registry.map((tag: Term) => ({ code: tag.code, label: tag[locale] })),
    language: [...new Set([...data.language_registry, ...criteria.language])].map(code => ({ code, label: languageLabel(code, locale) }))
      .sort((a, b) => a.label.localeCompare(b.label, locale)),
  }
  const chips = (['category', 'tags', 'language'] as const).flatMap(key => criteria[key].map(code => ({ key, code,
    label: options[key].find(option => option.code === code)?.label ?? code })))
  const ctx = { locale, region, interactions: data.interactions, tags: data.tag_registry }
  const active = Boolean(key)
  const canAppend = normalizeKeyword(query) === criteria.q && !changing
  return <SitesFrame page={page}>
    <section className={styles.directoryIntro}>
      <div><h1>{m.directory}</h1><p ref={resultHeading} tabIndex={-1} aria-live="polite">
        {m.results(data.total)}{accumulated ? ` · ${locale === 'zh' ? `已显示 ${results.length} 个` : `${results.length} shown`}` : ''}
      </p></div>
      <form className={styles.search} role="search" action={path} onSubmit={submit}>
        <MagnifyingGlassIcon size={21} aria-hidden="true" />
        {Object.entries(criteriaParams(criteria)).filter(([name, value]) => name !== 'q' && value).map(([name, value]) =>
          <input key={name} type="hidden" name={name} value={value} />)}
        <input ref={searchInput} type="search" name="q" aria-label={m.search} placeholder={m.placeholder} value={query} autoComplete="off"
          onChange={event => search(event.target.value)} onCompositionStart={() => { composing.current = true; clearTimeout(debounce.current); cancelPage() }}
          onCompositionEnd={event => { composing.current = false; search(event.currentTarget.value) }}
          onKeyDown={event => { if (event.key === 'Enter' && (composing.current || event.nativeEvent.isComposing)) event.preventDefault() }} />
        <button type="submit" className={styles.action}>{m.search}</button>
      </form>
    </section>
    <div className={styles.filterBar}>
      <div className={styles.quickFilters} role="group" aria-label={m.categories}>
        {options.category.map(option => <button className={styles.pill} key={option.code} type="button" aria-pressed={criteria.category.includes(option.code)}
          onClick={() => commit(toggle({ ...criteria, q: query }, 'category', option.code))}>{option.label}</button>)}
      </div>
      <div className={styles.filterActions}>
        <FilterDialog criteria={criteria} options={options} locale={locale} apply={draft => commit({ ...draft, q: query })} />
        <label className={`${styles.sort} ${styles.desktopSort}`}>{m.sort}<select className={styles.outline} value={data.sort}
          onChange={event => commit({ ...criteria, q: query, sort: event.target.value as Sort })}>
          {sorts.filter(sort => sort !== 'relevance' || criteria.q).map(sort => <option key={sort} value={sort}>{m[sort]}</option>)}
        </select></label>
      </div>
    </div>
    {active && <div className={styles.selected}>
      {criteria.q && <button type="button" className={styles.pill} onClick={() => { commit({ ...criteria, q: '' }); searchInput.current?.focus() }}
        aria-label={m.remove(criteria.q)}>{criteria.q}<XIcon size={12} aria-hidden="true" /></button>}
      {chips.map(chip => <button className={styles.pill} key={chip.key + chip.code} type="button"
        aria-label={m.remove(chip.label)} onClick={() => commit(toggle({ ...criteria, q: query }, chip.key, chip.code))}>
        {chip.label}<XIcon size={12} aria-hidden="true" />
      </button>)}
      <button type="button" className={styles.textAction} onClick={() => { commit(emptyCriteria); searchInput.current?.focus() }}>{m.reset}</button>
    </div>}
    {data.sort === 'heat_7d' && data.effective_sort === 'published_desc' && <p className={styles.notice} role="status">{m.fallback}</p>}
    <section aria-label={m.directory} aria-busy={loading || changing}>
      {results.length ? <div className={styles.directoryGrid}>{results.map(site => <SiteCard key={site.public_id} site={site} ctx={ctx} />)}</div>
        : <div className={styles.message}><h2>{m.empty}</h2><p>{m.emptyHint}</p></div>}
      {next && <div className={styles.pagination}>
        {issue && <p role="status">{issue === 'stale' ? m.stale : m.pageError}</p>}
        {issue === 'stale'
          ? <button type="button" className={styles.outline} onClick={() => void refresh()} disabled={loading}>{loading ? m.loading : m.refresh}</button>
          : <button type="button" className={styles.outline} data-load-more onClick={() => void loadMore()} disabled={loading || !canAppend}>
            {loading ? m.loading : issue ? m.retry : m.loadMore}
          </button>}
      </div>}
    </section>
  </SitesFrame>
}
