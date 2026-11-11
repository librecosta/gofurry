import { entityPath } from '../catalog/public-path'
import type { Locale, Region } from '../preferences'

export function yearbookPath(id: string, slug: string, year: number, locale: Locale, region: Region) {
  return `${entityPath('sites', id, slug, locale, region)}/yearbook/${year}`
}
