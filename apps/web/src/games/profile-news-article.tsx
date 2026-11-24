import type { Locale } from '../preferences'
import { ArrowUpRightIcon } from '@phosphor-icons/react/dist/ssr/ArrowUpRight'
import { profileCopy } from './profile-copy'
import { utcDate, type NewsArticle } from './profile-data'
import { RichHTML } from './profile-rich-details'
import styles from './profile.module.scss'

export function NewsArticleView({ article, locale }: { article: NewsArticle; locale: Locale }) {
  const m = profileCopy[locale]
  return <article id="news-article" className={styles.officialArticle} aria-labelledby="official-article-title" lang={article.locale}>
    <header>
      <h3 id="official-article-title">{article.title}</h3>
      <p className={styles.muted}><time dateTime={article.published_at}>{utcDate(article.published_at, locale)}</time>
        {article.updated_at && article.updated_at !== article.published_at && <> · {m.articleUpdated} <time dateTime={article.updated_at}>{utcDate(article.updated_at, locale)}</time></>}
        {article.locale !== locale && <> · {m.articleFallback}</>}</p>
    </header>
    <RichHTML html={article.html} />
    {!!article.tags?.length && <p className={styles.muted}>{article.tags.join(' · ')}</p>}
    <p className={styles.muted}>{[
      [m.articleComments, article.metrics?.comment_count], [m.articleVotesUp, article.metrics?.vote_up_count],
      [m.articleVotesDown, article.metrics?.vote_down_count],
    ].filter(([, count]) => typeof count === 'number').map(([label, count]) => label + ' ' + count).join(' · ')}</p>
    <a href={article.url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">
      {m.articleSource} <ArrowUpRightIcon size={15} aria-hidden="true" />
    </a>
  </article>
}
