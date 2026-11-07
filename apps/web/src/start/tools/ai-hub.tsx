import chatgpt from '../../assets/ai-hub/chatgpt.webp'
import claude from '../../assets/ai-hub/claude.webp'
import gemini from '../../assets/ai-hub/gemini.webp'
import deepseek from '../../assets/ai-hub/deepseek.webp'
import perplexity from '../../assets/ai-hub/perplexity.webp'
import grok from '../../assets/ai-hub/grok.webp'
import doubao from '../../assets/ai-hub/doubao.webp'
import kimi from '../../assets/ai-hub/kimi.webp'
import qwen from '../../assets/ai-hub/qwen.webp'
import yuanbao from '../../assets/ai-hub/yuanbao.webp'
import { NavigationHub, type HubLink } from './navigation-hub'

const links = [
  { name: { zh: 'ChatGPT', en: 'ChatGPT' }, icon: chatgpt, href: 'https://chatgpt.com/' },
  { name: { zh: 'Claude', en: 'Claude' }, icon: claude, href: 'https://claude.ai/' },
  { name: { zh: 'Gemini', en: 'Gemini' }, icon: gemini, href: 'https://gemini.google.com/' },
  { name: { zh: 'DeepSeek', en: 'DeepSeek' }, icon: deepseek, href: 'https://chat.deepseek.com/' },
  { name: { zh: 'Perplexity', en: 'Perplexity' }, icon: perplexity, href: 'https://www.perplexity.ai/' },
  { name: { zh: 'Grok', en: 'Grok' }, icon: grok, href: 'https://grok.com/' },
  { name: { zh: '豆包', en: 'Doubao' }, icon: doubao, href: 'https://www.doubao.com/' },
  { name: { zh: 'Kimi', en: 'Kimi' }, icon: kimi, href: 'https://www.kimi.com/' },
  { name: { zh: '千问', en: 'Qwen' }, icon: qwen, href: 'https://chat.qwen.ai/' },
  { name: { zh: '腾讯元宝', en: 'Yuanbao' }, icon: yuanbao, href: 'https://yuanbao.tencent.com/' },
] satisfies readonly HubLink[]

export default function AIHub() {
  return <NavigationHub label="AI Hub" links={links} />
}
