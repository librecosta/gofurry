import { createFileRoute } from '@tanstack/react-router'
import { detailHead, detailHeaders, loadDetail, MissingDetail } from '../catalog/detail'
import { SiteProfile } from '../catalog/site-profile'

export const Route = createFileRoute('/{-$language}/sites/$publicId/{-$slug}')({
  loader: {
    staleReloadMode: 'blocking',
    handler: ({ params, location, context }) => loadDetail('sites', params, location.pathname, context.preferences.region),
  },
  staleTime: 0,
  gcTime: 0,
  head: ({ loaderData }) => detailHead(loaderData),
  headers: ({ loaderData }) => detailHeaders(loaderData),
  notFoundComponent: MissingDetail,
  component: () => <SiteProfile data={Route.useLoaderData()} />,
})
