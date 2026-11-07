import videoGameInsights from '../../assets/game-data-hub/video-game-insights.webp?no-inline'
import steamdb from '../../assets/game-data-hub/steamdb.webp?no-inline'
import playtracker from '../../assets/game-data-hub/playtracker.webp?no-inline'
import gamalytic from '../../assets/game-data-hub/gamalytic.webp?no-inline'
import pcGamingWiki from '../../assets/game-data-hub/pc-gaming-wiki.webp?no-inline'
import { NavigationHub, type HubLink } from './navigation-hub'

const links = [
  { name: { zh: 'Video Game Insights', en: 'Video Game Insights' }, icon: videoGameInsights, href: 'https://app.sensortower.com/vgi' },
  { name: { zh: 'SteamDB', en: 'SteamDB' }, icon: steamdb, href: 'https://steamdb.info/' },
  { name: { zh: 'PlayTracker', en: 'PlayTracker' }, icon: playtracker, href: 'https://playtracker.net/insight/' },
  { name: { zh: 'Gamalytic', en: 'Gamalytic' }, icon: gamalytic, href: 'https://gamalytic.com/' },
  { name: { zh: 'PCGamingWiki', en: 'PCGamingWiki' }, icon: pcGamingWiki, href: 'https://www.pcgamingwiki.com/' },
] satisfies readonly HubLink[]

export default function GameDataHub() {
  return <NavigationHub label="Game Data Hub" links={links} />
}
