
import { createFileRoute } from '@tanstack/react-router'
import { MissingDetail } from '../catalog/detail'
import { loadGameProfile, profileHead, profileHeaders } from '../games/profile-load'
import { GameProfileView } from '../games/profile'

export const Route = createFileRoute('/{-$language}/games/$publicId/{-$slug}')({
  loader: ({ params, location, context, abortController }) => loadGameProfile(params, location.pathname, context.preferences.region, abortController.signal),
  head: ({ loaderData }) => profileHead(loaderData),
  headers: ({ loaderData }) => profileHeaders(loaderData),
  notFoundComponent: MissingDetail,
  component: () => {
    const page = Route.useLoaderData()
    return <GameProfileView key={page.id + '/' + page.locale} page={page} />
  },
})
