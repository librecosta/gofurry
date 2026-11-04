import { useEffect, useId, useRef, useState } from 'react'
import { CrosshairIcon } from '@phosphor-icons/react/dist/ssr/Crosshair'
import { usePreferences } from '../preferences'
import { useStartMessages } from './messages'
import styles from './weather.module.scss'

type City = { city_id: string; name: string; names: { zh: string; en: string }; admin: string; country_code: string }

export function WeatherCityPicker({ selected, onSelect, onBack }: { selected: string | null; onSelect: (id: string | null) => void; onBack: () => void }) {
  const { locale, region } = usePreferences()
  const m = useStartMessages().weather
  const [query, setQuery] = useState(''), [composing, setComposing] = useState(false)
  const [results, setResults] = useState<City[]>([]), [active, setActive] = useState(-1)
  const [status, setStatus] = useState('')
  const input = useRef<HTMLInputElement>(null), sequence = useRef(0), listId = useId()
  useEffect(() => { input.current?.focus() }, [])
  useEffect(() => {
    const version = ++sequence.current, controller = new AbortController()
    const q = query.trim()
    setResults([]); setActive(-1); setStatus('')
    if (!q || composing) return () => controller.abort()
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const response = await fetch(`/api/v1/start/cities?q=${encodeURIComponent(q)}`, { credentials: 'omit', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(4000)]) })
          if (!response.ok || response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') throw new Error('Unavailable')
          const value = await response.json() as { state?: string; cities?: City[] }
          if (!Array.isArray(value.cities) || value.cities.length > 10 || !value.cities.every(city => /^[1-9]\d{0,11}$/.test(city.city_id) && typeof city.name === 'string' && typeof city.names?.zh === 'string' && typeof city.names?.en === 'string' && typeof city.admin === 'string' && /^[A-Z]{2}$/.test(city.country_code))) throw new Error('Invalid cities')
          if (version === sequence.current) { setResults(value.cities); setActive(value.cities.length ? 0 : -1); setStatus(value.state === 'unavailable' ? m.cityUnavailable : value.cities.length ? '' : m.noCities) }
        } catch { if (version === sequence.current && !controller.signal.aborted) setStatus(m.cityUnavailable) }
      })()
    }, 300)
    return () => { ++sequence.current; clearTimeout(timer); controller.abort() }
  }, [query, composing, region, m.cityUnavailable, m.noCities])
  useEffect(() => { document.getElementById(`${listId}-${active}`)?.scrollIntoView({ block: 'nearest' }) }, [active, listId])
  return <div data-weather-city-picker onKeyDown={event => {
    if (event.key === 'Escape' && !event.nativeEvent.isComposing) { event.preventDefault(); event.stopPropagation(); onBack() }
  }}>
    <button className={styles.cityAction} type="button" onClick={() => onSelect(null)}><CrosshairIcon size={16} aria-hidden="true" />{m.automatic}</button>
    <input ref={input} className={styles.cityInput} value={query} maxLength={80} placeholder={m.citySearch} aria-label={m.citySearch}
      role="combobox" aria-autocomplete="list" aria-expanded={results.length > 0} aria-controls={listId} aria-activedescendant={active >= 0 ? `${listId}-${active}` : undefined}
      onChange={event => setQuery(event.target.value)} onCompositionStart={() => setComposing(true)} onCompositionEnd={() => setComposing(false)}
      onKeyDown={event => {
        if (composing || event.nativeEvent.isComposing) return
        if (results.length && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
          event.preventDefault(); setActive(index => (index + (event.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length)
        } else if (event.key === 'Enter' && active >= 0) { event.preventDefault(); onSelect(results[active].city_id) }
      }} />
    <div id={listId} role="listbox" aria-label={m.cityResults} className={styles.cityList}>
      {results.map((city, index) => <button key={city.city_id} id={`${listId}-${index}`} type="button" role="option" tabIndex={-1}
        aria-selected={active === index} data-selected={selected === city.city_id} data-active={active === index} className={styles.cityOption}
        onMouseEnter={() => setActive(index)} onMouseDown={event => event.preventDefault()} onClick={() => onSelect(city.city_id)}>
        <span>{city.names[locale] || city.name}</span><small>{city.admin} · {city.country_code}</small>
      </button>)}
    </div>
    <p className={styles.note} role="status">{status}</p>
    <p className={styles.note}>{m.citySource} <a className={styles.sourceLink} href="https://www.geonames.org" target="_blank" rel="noopener noreferrer">GeoNames</a></p>
  </div>
}
