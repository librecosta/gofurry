import { createFileRoute } from '@tanstack/react-router'
import { HomePage, SitesFailure, SitesFrame } from '../navigation/pages'
import { loadSites, sitesHead, sitesHeaders } from '../navigation/load'

export const Route = createFileRoute('/{-$language}/sites/')({
  loader: ({ params, location, context, abortController }) =>
    loadSites('home', params.language, location.pathname, location.searchStr, context.preferences.region, undefined, abortController.signal),
  head: ({ loaderData }) => sitesHead(loaderData),
  headers: ({ loaderData }) => sitesHeaders(loaderData),
  component: Sites,
})

function Sites() {
  const page = Route.useLoaderData()
  return page.state === 'ready' && 'featured' in page.data
    ? <HomePage page={page} home={page.data} />
    : <SitesFrame page={page}><SitesFailure page={page} /></SitesFrame>
}
