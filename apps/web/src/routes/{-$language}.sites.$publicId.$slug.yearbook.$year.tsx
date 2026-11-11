import { createFileRoute } from '@tanstack/react-router'
import { loadYearbook, MissingYearbook, yearbookHead, yearbookHeaders, YearbookView } from '../yearbook/page'

export const Route = createFileRoute('/{-$language}/sites/$publicId/$slug/yearbook/$year')({
  loader: ({ params, location, context }) => loadYearbook(params, location.pathname, context.preferences.region),
  head: ({ loaderData }) => yearbookHead(loaderData),
  headers: ({ loaderData }) => yearbookHeaders(loaderData),
  notFoundComponent: MissingYearbook,
  component: () => <YearbookView data={Route.useLoaderData()} />,
})
