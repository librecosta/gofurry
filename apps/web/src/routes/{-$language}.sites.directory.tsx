import { createFileRoute } from '@tanstack/react-router'
import { DirectoryPage } from '../navigation/directory'
import { normalizeCriteria } from '../navigation/data'
import { loadSites, sitesHead, sitesHeaders } from '../navigation/load'
import { SitesFailure } from '../navigation/pages'
import { SitesFrame } from '../navigation/frame'

export const Route = createFileRoute('/{-$language}/sites/directory')({
  loaderDeps: ({ search }) => normalizeCriteria(search),
  loader: {
    staleReloadMode: 'blocking',
    handler: ({ params, location, context, deps, abortController }) =>
      loadSites('directory', params.language, location.pathname, location.searchStr, context.preferences.region, deps, abortController.signal),
  },
  head: ({ loaderData }) => sitesHead(loaderData),
  headers: ({ loaderData }) => sitesHeaders(loaderData),
  component: Directory,
})

function Directory() {
  const page = Route.useLoaderData()
  return page.state === 'ready' && 'items' in page.data
    ? <DirectoryPage page={page} data={page.data} />
    : <SitesFrame page={page}><SitesFailure page={page} /></SitesFrame>
}
