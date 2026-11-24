import type { Locale } from '../preferences'
import styles from './profile.module.scss'

type Requirements = { minimum?: string; recommended?: string }
export type PublicGameRichDetails = {
  locale: Locale; fallback: boolean
  detailed_description?: string; about_the_game?: string; short_description?: string
  website?: string; support_info?: { url?: string; email?: string }
  pc_requirements?: Requirements; mac_requirements?: Requirements; linux_requirements?: Requirements
  languages?: { code: string; steam_name: string; interface?: boolean; subtitles?: boolean; full_audio?: boolean }[]
  required_age?: number; ratings?: Record<string, { rating?: string; required_age?: string }>
  content_descriptors?: { ids: number[] | null; notes?: string }
  platforms?: Partial<Record<'windows' | 'mac' | 'linux', boolean>>
  controller_support?: string
}

const messages = {
  en: { details: 'Detailed description', about: 'About the game', requirements: 'System requirements', minimum: 'Minimum',
    recommended: 'Recommended', contact: 'Official links', website: 'Website', support: 'Support', languages: 'Languages',
    language: 'Language', interface: 'Interface', subtitles: 'Subtitles', audio: 'Full audio', yes: 'Supported', no: 'Not supported',
    unknown: 'Unknown', ratings: 'Steam ratings and content', age: 'Required age', descriptors: 'Content descriptor IDs',
    platforms: 'Platform support', controller: 'Controller support', fallback: 'Published in another language' },
  zh: { details: '详细介绍', about: '关于游戏', requirements: '系统需求', minimum: '最低配置',
    recommended: '推荐配置', contact: '官方链接', website: '游戏官网', support: '客服支持', languages: '语言支持',
    language: '语言', interface: '界面', subtitles: '字幕', audio: '完整配音', yes: '支持', no: '不支持',
    unknown: '未知', ratings: 'Steam 评级与内容描述', age: '最低年龄', descriptors: '内容描述代码',
    platforms: '平台支持', controller: '控制器支持', fallback: '显示另一语言的已发布内容' },
}

function RichHTML({ html }: { html: string }) {
  // The public DTO contains the canonical, reviewed GoFurry HTML snapshot.
  return <div className={styles.richBody} dangerouslySetInnerHTML={{ __html: html }} />
}

function External({ href, label }: { href: string; label: string }) {
  return <a href={href} rel="noopener noreferrer nofollow" referrerPolicy="no-referrer">{label}</a>
}

export function ProfileRichDetails({ value, locale }: { value: PublicGameRichDetails; locale: Locale }) {
  const m = messages[locale]
  const requirements = [['Windows', value.pc_requirements], ['macOS', value.mac_requirements], ['Linux', value.linux_requirements]] as const
  const support = (state?: boolean) => state === undefined ? m.unknown : state ? m.yes : m.no
  const ratings = Object.entries(value.ratings ?? {}).filter(([, entry]) => entry.rating || entry.required_age)
  const platforms = [['Windows', value.platforms?.windows], ['macOS', value.platforms?.mac], ['Linux', value.platforms?.linux]] as const
  const descriptors = value.content_descriptors?.ids
  const links = value.website || value.support_info?.url || value.support_info?.email
  return <section className={styles.section} id="published-details">
    {value.fallback && <p className={styles.muted}>{m.fallback} ({value.locale})</p>}
    <div lang={value.locale}>
      {value.detailed_description && <><h2>{m.details}</h2><RichHTML html={value.detailed_description} /></>}
      {value.about_the_game && value.about_the_game !== value.detailed_description && <><h2>{m.about}</h2><RichHTML html={value.about_the_game} /></>}
      {!value.detailed_description && !value.about_the_game && value.short_description && <><h2>{m.about}</h2><RichHTML html={value.short_description} /></>}
      {requirements.some(([, r]) => r?.minimum || r?.recommended) && <>
        <h2>{m.requirements}</h2>
        <div className={styles.richRequirements}>{requirements.map(([platform, r]) => r && (r.minimum || r.recommended) && <article key={platform}>
          <h3>{platform}</h3>
          {r.minimum && <><h4>{m.minimum}</h4><RichHTML html={r.minimum} /></>}
          {r.recommended && <><h4>{m.recommended}</h4><RichHTML html={r.recommended} /></>}
        </article>)}</div>
      </>}
      {!!value.languages?.length && <><h2>{m.languages}</h2>
        <div className={styles.richTable}><table aria-label={m.languages}>
          <thead><tr>{[m.language, m.interface, m.subtitles, m.audio].map(label => <th key={label} scope="col">{label}</th>)}</tr></thead>
          <tbody>{value.languages.map((language, index) => <tr key={language.code + '/' + index}>
            <th scope="row">{language.steam_name}</th><td>{support(language.interface)}</td>
            <td>{support(language.subtitles)}</td><td>{support(language.full_audio)}</td>
          </tr>)}</tbody>
        </table></div>
      </>}
      {links && <><h2>{m.contact}</h2><ul>
        {value.website && <li><External href={value.website} label={m.website} /></li>}
        {value.support_info?.url && <li><External href={value.support_info.url} label={m.support} /></li>}
        {value.support_info?.email && <li>{value.support_info.email}</li>}
      </ul></>}
      {(value.platforms || value.controller_support) && <><h2>{m.platforms}</h2><ul>
        {value.platforms && platforms.map(([platform, state]) => <li key={platform}>{platform} · {support(state)}</li>)}
        {value.controller_support && <li>{m.controller} · {value.controller_support}</li>}
      </ul></>}
      {(ratings.length > 0 || value.required_age !== undefined || descriptors?.length || value.content_descriptors?.notes) && <>
        <h2>{m.ratings}</h2><ul>
          {value.required_age !== undefined && <li>{m.age} {value.required_age}</li>}
          {!!descriptors?.length && <li>{m.descriptors} {descriptors.join(', ')}</li>}
          {ratings.map(([board, rating]) => <li key={board}>{board.replaceAll('_', ' ')} {[rating.rating, rating.required_age].filter(Boolean).join(' · ')}</li>)}
        </ul>
        {value.content_descriptors?.notes && <RichHTML html={value.content_descriptors.notes} />}
      </>}
    </div>
  </section>
}
