import { NavigationHub, type HubLink } from './navigation-hub'

const links = [
  { name: { zh: 'ChatGPT', en: 'ChatGPT' }, mark: 'CG', href: 'https://chatgpt.com/' },
  { name: { zh: 'Claude', en: 'Claude' }, mark: 'C', href: 'https://claude.ai/' },
  { name: { zh: 'Gemini', en: 'Gemini' }, mark: 'G', href: 'https://gemini.google.com/' },
  { name: { zh: 'DeepSeek', en: 'DeepSeek' }, mark: 'DS', href: 'https://chat.deepseek.com/' },
  { name: { zh: 'Perplexity', en: 'Perplexity' }, mark: 'P', href: 'https://www.perplexity.ai/' },
  { name: { zh: 'Grok', en: 'Grok' }, mark: 'X', href: 'https://grok.com/' },
  { name: { zh: '豆包', en: 'Doubao' }, mark: 'DB', href: 'https://www.doubao.com/' },
  { name: { zh: 'Kimi', en: 'Kimi' }, mark: 'K', href: 'https://www.kimi.com/' },
  { name: { zh: '千问', en: 'Qwen' }, mark: 'Q', href: 'https://chat.qwen.ai/' },
  { name: { zh: '腾讯元宝', en: 'Yuanbao' }, mark: 'Y', href: 'https://yuanbao.tencent.com/' },
] satisfies readonly HubLink[]

export default function AIHub() {
  return <NavigationHub label="AI Hub" links={links} />
}
