import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Popover } from '@base-ui/react/popover'
import { AnimatePresence, motion, useIsPresent } from 'motion/react'
import { MapPinIcon } from '@phosphor-icons/react/dist/ssr/MapPin'
import { usePreferences } from '../preferences'
import { useMotionTiming } from '../motion'
import { useStartMessages } from './messages'
import { failedWeatherRefresh, parseWeather, safeAttributionURL, unavailableWeather, weatherRefreshAge, weatherTemperature, type WeatherData } from './weather-data'
import styles from './weather.module.scss'
import controls from './controls.module.scss'
import { cx } from '../styles/classes'
import { WeatherCityPicker } from './weather-city-picker'
import { WeatherIcon } from './weather-icon'

function useWeather() {
  const { locale, region } = usePreferences()
  const [data, setData] = useState<WeatherData>(unavailableWeather)
  const [cityId, setCityId] = useState<string | null | undefined>(undefined)
  const generation = useRef(0)
  const key = `gf-weather-city:${region}`
  useEffect(() => {
    let saved: string | null = null
    try { const value = localStorage.getItem(key); if (value && /^[1-9]\d{0,11}$/.test(value)) saved = value } catch { /* Storage can be disabled. */ }
    setCityId(saved)
    if (saved) setData({ ...unavailableWeather, locationSource: 'manual', cityId: saved })
  }, [key])
  useEffect(() => {
    if (cityId === undefined) return
    let controller: AbortController | undefined
    let lastSuccess = 0
    let pending = false
    const load = async () => {
      if (pending) return
      pending = true
      controller = new AbortController()
      const request = ++generation.current
      try {
        const response = await fetch(`/api/v1/start/weather?lang=${locale}${cityId ? `&city_id=${cityId}` : ''}`, {
          credentials: 'omit', redirect: 'error', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(6000)]),
        })
        if (!response.ok || response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') throw new Error('Weather unavailable')
        const next = parseWeather(await response.json())
        if (next.locationSource !== (cityId ? 'manual' : 'ip') || next.cityId !== cityId) throw new Error('Location mismatch')
        if (generation.current === request) { lastSuccess = Date.now(); setData(next) }
      } catch {
        if (generation.current === request) setData(current => failedWeatherRefresh(current, Date.now()))
      } finally { pending = false }
    }
    void load()
    const onVisible = () => {
      if (document.visibilityState === 'visible' && Date.now() - lastSuccess >= weatherRefreshAge) void load()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      ++generation.current
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [locale, region, cityId])
  const selectCity = (id: string | null) => {
    if (id === cityId) return
    ++generation.current
    try { if (id) localStorage.setItem(key, id); else localStorage.removeItem(key) } catch { /* In-memory selection still works. */ }
    setData({ ...unavailableWeather, locationSource: id ? 'manual' : 'ip', cityId: id })
    setCityId(id)
  }
  return { data, cityId: cityId ?? null, selectCity }
}

export function StartWeather() {
  const { locale } = usePreferences()
  const m = useStartMessages().weather
  const { data, cityId, selectCity } = useWeather()
  const [open, setOpen] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const cityButton = useRef<HTMLButtonElement>(null)
  const restoreCityFocus = useRef(false)
  const transition = useMotionTiming()
  const city = data.city?.[locale]
  const temperature = data.weather ? weatherTemperature(data.weather.temperature_c, locale) : null
  const label = temperature ? `${city}, ${data.weather!.condition_text}, ${temperature}` : m.unavailable
  const back = () => { restoreCityFocus.current = true; setSelecting(false) }
  // A provider name/URL already visible on the source line needs no duplicate
  // row. Other actual targets remain inline, and required declarations stay intact.
  const attributionLinks = [...new Set(data.attributions.map(safeAttributionURL).filter((url): url is string => !!url && url !== 'https://www.qweather.com/' && url !== 'https://developer.qweather.com/attribution.html'))]
  const attributionText = data.attributions.filter(text => !safeAttributionURL(text) && !['qweather', '和风天气', '和风天气 / qweather'].includes(text.trim().toLowerCase()))
  return <Popover.Root open={open} onOpenChange={value => { setOpen(value); if (value) { setSelecting(false); restoreCityFocus.current = false } }}>
    <Popover.Trigger data-start-weather data-weather-state={data.state} aria-label={`${m.details}: ${label}`} className={cx(controls.header, styles.trigger)}>
      <span className={temperature ? styles.city : undefined}>{temperature ? city : m.title}</span>
      <WeatherIcon code={data.weather?.condition_code} />
      {temperature && <span className={styles.temperature}>{temperature}</span>}
    </Popover.Trigger>
    <AnimatePresence>{open && <Popover.Portal keepMounted>
      <Popover.Positioner positionMethod="fixed" align="end" sideOffset={8} collisionPadding={16} className={styles.positioner}>
        <Popover.Popup data-start-weather-details className={styles.popup} render={<motion.div
          initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }} transition={transition}
        />}>
          <Popover.Title className={styles.title}>{selecting ? m.selectCity : city ?? m.title}</Popover.Title>
          <div className={styles.views}><AnimatePresence initial={false}>
          {selecting ? <WeatherView key="cities">
            <WeatherCityPicker selected={cityId} active={open} locationSource={data.locationSource} onBack={back} onSelect={id => { selectCity(id); back() }} />
          </WeatherView> : <WeatherView key="details" onEnter={() => { if (open && restoreCityFocus.current) { cityButton.current?.focus(); restoreCityFocus.current = false } }}>
          {data.weather ? <>
            <div className={styles.current}><WeatherIcon code={data.weather.condition_code} size={28} /><span>{temperature}</span></div>
            <Popover.Description className={styles.description}>{data.weather.condition_text}</Popover.Description>
            {data.state === 'stale' && <p className={styles.note}>{m.stale}</p>}
            <p className={styles.note}>{m.fetched} <time dateTime={data.weather.fetched_at}>{new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en', {
              month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
            }).format(new Date(data.weather.fetched_at))}</time></p>
          </> : <Popover.Description className={styles.description}>{m.unavailable}</Popover.Description>}
          <div className={styles.sources}>
            <p>{m.weatherSource} <a href="https://www.qweather.com" target="_blank" rel="noopener noreferrer">{m.provider}</a>{attributionLinks.map(url => <span key={url}> · <a href={url} target="_blank" rel="noopener noreferrer">{new URL(url).hostname}</a></span>)}</p>
            {attributionText.length > 0 && <ul>{attributionText.map((text,index) => <li key={`${index}:${text}`}>{text}</li>)}</ul>}
          </div>
          <button ref={cityButton} type="button" className={styles.cityAction} onClick={() => setSelecting(true)}><MapPinIcon size={16} aria-hidden="true" />{m.selectCity}</button>
          </WeatherView>}
          </AnimatePresence></div>
        </Popover.Popup>
      </Popover.Positioner>
    </Popover.Portal>}</AnimatePresence>
  </Popover.Root>
}

// Both views share one grid cell during the 420ms crossfade. Exiting content
// keeps its measured footprint until removal, but cannot receive focus/clicks.
function WeatherView({ children, onEnter }: { children: ReactNode; onEnter?: () => void }) {
  const present = useIsPresent()
  const transition = useMotionTiming()
  useEffect(() => { if (present) onEnter?.() }, [present, onEnter])
  return <motion.div className={styles.view} inert={!present} aria-hidden={!present || undefined}
    initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 4 }} transition={transition}>
    {children}
  </motion.div>
}
