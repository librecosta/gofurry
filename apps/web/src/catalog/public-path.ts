import { localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'
import type { Kind } from './detail-server'

export function entityPath(kind: Kind, id: string, slug: string, locale: Locale, region: Region) {
  return localizedPath(`/${kind}/${id}/${slug}`, locale, region)
}
