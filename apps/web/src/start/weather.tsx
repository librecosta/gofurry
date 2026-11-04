import { useEffect, useRef, useState } from 'react'
import { Popover } from '@base-ui/react/popover'
import { AnimatePresence, motion } from 'motion/react'
import { SunIcon } from '@phosphor-icons/react/dist/ssr/Sun'
import { CloudSunIcon } from '@phosphor-icons/react/dist/ssr/CloudSun'
import { CloudIcon } from '@phosphor-icons/react/dist/ssr/Cloud'
import { CloudRainIcon } from '@phosphor-icons/react/dist/ssr/CloudRain'
import { CloudSnowIcon } from '@phosphor-icons/react/dist/ssr/CloudSnow'
import { CloudFogIcon } from '@phosphor-icons/react/dist/ssr/CloudFog'
import { CloudLightningIcon } from '@phosphor-icons/react/dist/ssr/CloudLightning'
import { CloudSlashIcon } from '@phosphor-icons/react/dist/ssr/CloudSlash'
import { ThermometerIcon } from '@phosphor-icons/react/dist/ssr/Thermometer'
import { WindIcon } from '@phosphor-icons/react/dist/ssr/Wind'
import { QuestionIcon } from '@phosphor-icons/react/dist/ssr/Question'
import { usePreferences } from '../preferences'
import { useMotionTiming } from '../motion'
import { useStartMessages } from './messages'
import { failedWeatherRefresh, parseWeather, safeAttributionURL, unavailableWeather, weatherRefreshAge, weatherTemperature, type WeatherData } from './weather-data'
import styles from './weather.module.scss'

// Documented common conditions only; future/unknown codes stay neutral.
// https://dev.qweather.com/docs/api/weather/weather-conditions/
function conditionIcon(code?: string) {
  if (!code) return CloudSlashIcon
  if (code === '100') return SunIcon
  if (['101', '102', '103'].includes(code)) return CloudSunIcon
  if (code === '104') return CloudIcon
  if (['302', '303', '304'].includes(code)) return CloudLightningIcon
  if (['300', '301', '305', '306', '307', '308', '309', '310', '311', '312', '313', '314', '315', '316', '317', '318', '399'].includes(code)) return CloudRainIcon
  if (['400', '401', '402', '403', '404', '405', '406', '407', '408', '409', '410', '499'].includes(code)) return CloudSnowIcon
  if (['500', '501', '502', '509', '510', '511', '512', '513', '514', '515'].includes(code)) return CloudFogIcon
  if (['503', '504', '507', '508'].includes(code)) return WindIcon
  if (['900', '901'].includes(code)) return ThermometerIcon
  return QuestionIcon
}

function useWeather() {
  const { locale, region } = usePreferences()
  const [data, setData] = useState<WeatherData>(unavailableWeather)
  const generation = useRef(0)
  useEffect(() => {
    let controller: AbortController | undefined
    let lastSuccess = 0
    let pending = false
    const load = async () => {
      if (pending) return
      pending = true
      controller = new AbortController()
      const request = ++generation.current
      try {
        const response = await fetch(`/api/v1/start/weather?lang=${locale}`, {
          credentials: 'omit', redirect: 'error', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(6000)]),
        })
        if (!response.ok || response.headers.get('x-gofurry-region') !== region || response.headers.get('x-gofurry-content-mode') !== 'SFW') throw new Error('Weather unavailable')
        const next = parseWeather(await response.json())
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
  }, [locale, region])
  return data
}

export function StartWeather() {
  const { locale } = usePreferences()
  const m = useStartMessages().weather
  const data = useWeather()
  const [open, setOpen] = useState(false)
  const transition = useMotionTiming()
  const city = data.city?.[locale]
  const temperature = data.weather ? weatherTemperature(data.weather.temperature_c, locale) : null
  const Icon = conditionIcon(data.weather?.condition_code)
  const label = temperature ? `${city}, ${data.weather!.condition_text}, ${temperature}` : m.unavailable
  return <Popover.Root open={open} onOpenChange={setOpen}>
    <Popover.Trigger data-start-weather data-weather-state={data.state} aria-label={`${m.details}: ${label}`} className={styles.trigger}>
      <span className={styles.city}>{city ?? m.title}</span>
      <Icon size={20} aria-hidden="true" data-weather-icon={data.weather?.condition_code ?? 'unavailable'} />
      <span className={styles.temperature}>{temperature ?? '—'}</span>
    </Popover.Trigger>
    <AnimatePresence>{open && <Popover.Portal keepMounted>
      <Popover.Positioner positionMethod="fixed" align="end" sideOffset={8} collisionPadding={16} className={styles.positioner}>
        <Popover.Popup data-start-weather-details className={styles.popup} render={<motion.div
          initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }} transition={transition}
        />}>
          <Popover.Title className={styles.title}>{city ?? m.title}</Popover.Title>
          {data.weather ? <>
            <div className={styles.current}><Icon size={28} aria-hidden="true" /><span>{temperature}</span></div>
            <Popover.Description className={styles.description}>{data.weather.condition_text}</Popover.Description>
            {data.state === 'stale' && <p className={styles.note}>{m.stale}</p>}
            <p className={styles.note}>{m.fetched} <time dateTime={data.weather.fetched_at}>{new Intl.DateTimeFormat(locale === 'zh' ? 'zh-CN' : 'en', {
              month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
            }).format(new Date(data.weather.fetched_at))}</time></p>
          </> : <Popover.Description className={styles.description}>{m.unavailable}</Popover.Description>}
          <div className={styles.sources}>
            <p>{m.weatherSource} <a href="https://www.qweather.com" target="_blank" rel="noopener noreferrer">{m.provider}</a></p>
            <p>{m.locationSource} <a href="https://db-ip.com" target="_blank" rel="noopener noreferrer">DB-IP</a></p>
            {data.attributions.length > 0 && <ul>{data.attributions.map((text, index) => {
              const url = safeAttributionURL(text)
              return <li key={`${index}:${text}`}>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{text}</a> : text}</li>
            })}</ul>}
          </div>
        </Popover.Popup>
      </Popover.Positioner>
    </Popover.Portal>}</AnimatePresence>
  </Popover.Root>
}
