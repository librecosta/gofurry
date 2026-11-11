import { createFileRoute } from '@tanstack/react-router'
import { detailHead, detailHeaders, DetailView, loadDetail, MissingDetail } from '../catalog/detail'

export const Route = createFileRoute('/{-$language}/games/$publicId/{-$slug}')({
  loader: ({ params, location, context }) => loadDetail('games', params, location.pathname, context.preferences.region),
  head: ({ loaderData }) => detailHead(loaderData),
  headers: ({ loaderData }) => detailHeaders(loaderData),
  notFoundComponent: MissingDetail,
  component: () => <DetailView data={Route.useLoaderData()} />,
})
