import pixabay from '../../assets/asset-hub/pixabay.webp?no-inline'
import pexels from '../../assets/asset-hub/pexels.webp?no-inline'
import polyHaven from '../../assets/asset-hub/poly-haven.webp?no-inline'
import ambientcg from '../../assets/asset-hub/ambientcg.webp?no-inline'
import kenney from '../../assets/asset-hub/kenney.webp?no-inline'
import quaternius from '../../assets/asset-hub/quaternius.webp?no-inline'
import openGameArt from '../../assets/asset-hub/open-game-art.webp?no-inline'
import freesound from '../../assets/asset-hub/freesound.webp?no-inline'
import { NavigationHub, type HubLink } from './navigation-hub'

const links = [
  { name: { zh: 'Pixabay', en: 'Pixabay' }, icon: pixabay, href: 'https://pixabay.com/' },
  { name: { zh: 'Pexels', en: 'Pexels' }, icon: pexels, href: 'https://www.pexels.com/' },
  { name: { zh: 'Poly Haven', en: 'Poly Haven' }, icon: polyHaven, href: 'https://polyhaven.com/' },
  { name: { zh: 'ambientCG', en: 'ambientCG' }, icon: ambientcg, href: 'https://ambientcg.com/' },
  { name: { zh: 'Kenney', en: 'Kenney' }, icon: kenney, href: 'https://kenney.nl/assets' },
  { name: { zh: 'Quaternius', en: 'Quaternius' }, icon: quaternius, href: 'https://quaternius.com/' },
  { name: { zh: 'OpenGameArt', en: 'OpenGameArt' }, icon: openGameArt, href: 'https://opengameart.org/' },
  { name: { zh: 'Freesound', en: 'Freesound' }, icon: freesound, href: 'https://freesound.org/' },
] satisfies readonly HubLink[]

export default function AssetHub() {
  return <NavigationHub label="Asset Hub" links={links} />
}
