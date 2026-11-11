import { createFileRoute } from '@tanstack/react-router'
import { detailHead, detailHeaders, DetailView, loadDetail, MissingDetail } from '../catalog/detail'

export const Route = createFileRoute('/{-$language}/sites/$publicId/{-$slug}')({
  loader: ({ params, location, context }) => loadDetail('sites', params, location.pathname, context.preferences.region),
  head: ({ loaderData }) => detailHead(loaderData),
  headers: ({ loaderData }) => detailHeaders(loaderData),
  notFoundComponent: MissingDetail,
  component: () => <DetailView data={Route.useLoaderData()} />,
})
