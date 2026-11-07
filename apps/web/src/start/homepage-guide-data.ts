import { localizedPath } from '../locale-path'
import type { Locale, Region } from '../preferences'

export const homepageBrowsers = ['Edge', 'Chrome', 'Firefox', 'Safari'] as const
export type HomepageBrowser = typeof homepageBrowsers[number] | 'other'

export function detectHomepageBrowser(userAgent: string): HomepageBrowser {
  if (/\b(?:Android|iPhone|iPad|iPod)\b/.test(userAgent)) return 'other'
  if (/\bEdg\//.test(userAgent)) return 'Edge'
  if (/\bFirefox\//.test(userAgent)) return 'Firefox'
  if (/\b(?:OPR|Opera|Vivaldi|YaBrowser|SamsungBrowser|Chromium)\//.test(userAgent)) return 'other'
  if (/\b(?:Headless)?Chrome\//.test(userAgent)) return 'Chrome'
  if (/\bMacintosh\b.*\bVersion\/.*\bSafari\//.test(userAgent)) return 'Safari'
  return 'other'
}

const deploymentOrigins = { CN: 'https://gofurry.cn', INTL: 'https://gofurry.com' }

export function homepageAddress(region: Region, locale: Locale, currentOrigin: string) {
  const origin = URL.canParse(currentOrigin) ? new URL(currentOrigin) : null
  const local = !!origin && origin.origin === currentOrigin && ['127.0.0.1', 'localhost', '[::1]'].includes(origin.hostname)
    && (origin.protocol === 'http:' || origin.protocol === 'https:')
  return { url: new URL(localizedPath('/start', locale, region), local ? currentOrigin : deploymentOrigins[region]).href, local }
}

export const homepageMessages = {
  zh: {
    title: '设为首页',
    homepage: '浏览器首页',
    newTab: '新标签页',
    browser: '浏览器',
    address: 'GoFurry 起始页地址',
    localAddress: '本地开发地址',
    copy: '复制地址',
    copied: '地址已复制。',
    copyFailed: '复制失败，请手动复制地址。',
    hide: '不再显示入口',
    homeSteps: {
      Edge: ['打开「设置 → 开始、主页和新建标签页」。', '在「Edge 启动时」添加下方网址，或将其填入「主页按钮」的自定义网址。'],
      Chrome: ['打开「设置 → 启动时」，选择「打开特定网页或一组网页」，添加下方网址。', '也可在「外观」中开启「显示主页按钮」，并填入自定义网址。'],
      Firefox: ['打开「设置 → 主页和启动」（部分版本为「主页」）。', '在「主页」（或「主页和新窗口」）选择自定义网址，填入下方地址。若要启动时显示主页，请关闭上次会话恢复。'],
      Safari: ['在 macOS 上打开「Safari 浏览器 → 设置 → 通用」，将下方网址填入「主页」。', '将「新建窗口时打开」设为「主页」。如启用了会话恢复，启动时仍可能打开上次的窗口。'],
      other: ['在浏览器设置中查找「主页」或「启动时」。如果支持自定义网址，请填入下方地址。'],
    },
    newTabSteps: {
      Safari: ['在 macOS 上打开「Safari 浏览器 → 设置 → 通用」，将下方网址填入「主页」。', '将「新建标签页时打开」设为「主页」。'],
      other: ['在浏览器设置中查找「新标签页」。如果提供自定义网址选项，请填入下方地址。'],
    },
    extension: '桌面版的常规设置不提供自定义新标签页网址，通常需要浏览器扩展。你仍可将 GoFurry 设为首页或启动页。',
  },
  en: {
    title: 'Set as homepage',
    homepage: 'Browser homepage',
    newTab: 'New tab',
    browser: 'Browser',
    address: 'GoFurry start page address',
    localAddress: 'Local development address',
    copy: 'Copy address',
    copied: 'Address copied.',
    copyFailed: 'Could not copy. Copy the address manually.',
    hide: 'Hide this shortcut',
    homeSteps: {
      Edge: ['Open Settings → Start, home, and new tabs.', 'Add the address below under When Edge starts, or enter it as the custom URL for the Home button.'],
      Chrome: ['Open Settings → On startup, select the option to open specific pages, and add the address below.', 'You can also enable the Home button under Appearance and enter a custom address.'],
      Firefox: ['Open Settings → Home and startup (Home in some versions).', 'Choose a custom URL under Homepage (or Homepage and new windows) and enter the address below. To see it at startup, turn off session restore.'],
      Safari: ['On macOS, open Safari → Settings → General and enter the address below in Homepage.', 'Set New windows open with to Homepage. Safari may still restore your previous windows at startup if session restore is enabled.'],
      other: ['Look for Homepage or On startup in your browser settings. If a custom URL is available, enter the address below.'],
    },
    newTabSteps: {
      Safari: ['On macOS, open Safari → Settings → General and enter the address below in Homepage.', 'Set New tabs open with to Homepage.'],
      other: ['Look for New tab in your browser settings. If a custom URL is available, enter the address below.'],
    },
    extension: 'Standard desktop settings do not offer a custom new-tab URL. This usually requires a browser extension. You can still set GoFurry as your homepage or startup page.',
  },
}
