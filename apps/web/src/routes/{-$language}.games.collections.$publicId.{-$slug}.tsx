import { createFileRoute } from '@tanstack/react-router'
import { gameHead, gameHeaders, loadGames } from '../games/load'
import { GameChannelPage, MissingGamePage } from '../games/pages'

export const Route = createFileRoute('/{-$language}/games/collections/$publicId/{-$slug}')({
  loaderDeps: ({ search }) => search,
  loader: {
    staleReloadMode: 'blocking',
    handler: ({ params, location, context, abortController }) =>
      loadGames('members', params, location.pathname, location.searchStr, context.preferences.region, abortController.signal),
  },
  head: ({ loaderData }) => gameHead(loaderData),
  headers: ({ loaderData }) => gameHeaders(loaderData),
  notFoundComponent: MissingGamePage,
  component: () => <GameChannelPage page={Route.useLoaderData()} />,
})
